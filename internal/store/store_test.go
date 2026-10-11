package store_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// The PostgreSQL contract (ADR-010/ADR-006): these tests run against a real
// database pointed at by PATCHCASTER_TEST_DATABASE_URL and RECREATE its
// public schema. Point them only at a disposable database. Without the
// variable they skip, keeping offline unit runs green.
func openTestStore(t *testing.T) (*store.Postgres, *sql.DB) {
	t.Helper()
	url := os.Getenv("PATCHCASTER_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PATCHCASTER_TEST_DATABASE_URL not set")
	}
	cfg, err := pgx.ParseConfig(url)
	require.NoError(t, err)
	db := pgxstdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	_, err = db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err, "test database must be disposable")

	// Migrate through the production path (provider + session locker).
	applied, err := store.Migrate(ctx, db)
	require.NoError(t, err)
	require.Positive(t, applied, "fresh schema must apply at least one migration")

	return store.New(db), db
}

func newEvent(id string) *event.Event {
	ev := &event.Event{
		WebhookID: id, Kind: event.KindMergeRequest, Action: "open",
		MRRef: "1234", IID: 3, ProjectPath: "grp/app", Title: "T",
		Author: "ada", Actor: "ada", URL: "https://x/1",
		SourceBranch: "feat", TargetBranch: "main", Open: true,
		When: time.Now().UTC(),
	}
	ev.Plan = plan.For(ev) // the ingest edge does this before Accept; tests mirror it
	return ev
}

// enqueueAll accepts each delivery in order and requires a queue row for it.
func enqueueAll(t *testing.T, st *store.Postgres, at time.Time, events ...*event.Event) {
	t.Helper()
	for _, ev := range events {
		res, err := st.Accept(context.Background(), ev, "chan-1", at)
		require.NoError(t, err)
		require.True(t, res.Enqueued, "delivery %q must enqueue", ev.WebhookID)
	}
}

// claimDelivery claims the next due job and names it by webhook identifier.
// It returns nil and an empty name when nothing is claimable.
func claimDelivery(t *testing.T, st *store.Postgres, at time.Time) (*store.Job, string) {
	t.Helper()
	job, err := st.ClaimDue(context.Background(), at, time.Minute)
	require.NoError(t, err)
	if job == nil {
		return nil, ""
	}
	ev, err := job.Event()
	require.NoError(t, err)
	return job, ev.WebhookID
}

func TestPostgres_AcceptDedupeAndClaim(t *testing.T) {
	st, db := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// accept durably pairs seen-mark with the queue row
	res, err := st.Accept(ctx, newEvent("msg-1"), "chan-1", now)
	require.NoError(t, err)
	assert.True(t, res.Accepted)

	seen, err := st.Seen(ctx, "msg-1")
	require.NoError(t, err)
	assert.True(t, seen)

	// a replay of the same identifier is refused without a second enqueue
	res, err = st.Accept(ctx, newEvent("msg-1"), "chan-1", now)
	require.NoError(t, err)
	assert.False(t, res.Accepted, "the duplicate must not enqueue again")
	assert.False(t, res.Enqueued)

	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM outbox").Scan(&count))
	assert.Equal(t, 1, count, "dedupe must not leave a second queue row")

	// claim takes it with the lease stamped
	job, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	got, err := job.Event()
	require.NoError(t, err)
	assert.Equal(t, "msg-1", got.WebhookID)
	assert.Equal(t, "chan-1", job.DestChannel)

	// nothing else is pending while it is claimed
	job, err = st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	assert.Nil(t, job)

	// lease expiry returns it to the pool
	require.NoError(t, st.ReapLeases(ctx, now.Add(2*time.Minute)))
	job, err = st.ClaimDue(ctx, now.Add(2*time.Minute), time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job, "reaped lease must requeue the job")

	// and settlement closes it
	require.NoError(t, st.Settle(ctx, job.ID, now.Add(2*time.Minute), store.Sent()))
	job, err = st.ClaimDue(ctx, now.Add(3*time.Minute), time.Minute)
	require.NoError(t, err)
	assert.Nil(t, job, "sent jobs never return to the queue")
}

func TestPostgres_SettleVariantsAndRetry(t *testing.T) {
	st, db := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	later := now.Add(time.Hour + time.Second)

	// scenario 1: retry defers, then the job returns due with attempts kept
	_, err := st.Accept(ctx, newEvent("r-1"), "chan-1", now)
	require.NoError(t, err)
	job, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, st.Settle(ctx, job.ID, now,
		store.Retried(1, now.Add(time.Hour), "mattermost said no")))

	job, err = st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	assert.Nil(t, job, "a deferred job must not be due")

	job, err = st.ClaimDue(ctx, later, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job, "the job returns once due")
	assert.Equal(t, 1, job.Attempts)
	require.NoError(t, st.Settle(ctx, job.ID, later, store.Sent()))

	// scenario 2: parked leaves the retry rotation permanently
	_, err = st.Accept(ctx, newEvent("r-2"), "chan-1", now)
	require.NoError(t, err)
	job, err = st.ClaimDue(ctx, later, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, st.Settle(ctx, job.ID, later,
		store.Parked("attempt_budget", 8, "http 500")))

	// scenario 3: skipped is terminal without any Mattermost contact
	_, err = st.Accept(ctx, newEvent("r-3"), "chan-1", now)
	require.NoError(t, err)
	job, err = st.ClaimDue(ctx, later, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, st.Settle(ctx, job.ID, later, store.Skipped("no_mapping")))

	var states []string
	rows, err := db.QueryContext(ctx,
		"SELECT state || ':' || COALESCE(settled_reason,'') FROM outbox ORDER BY id")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		states = append(states, s)
	}
	assert.Equal(t, []string{"sent:", "parked:attempt_budget", "skipped:no_mapping"}, states)
}

// A deferred delivery holds back the rest of its merge request. A comment
// claimed ahead of its retrying root would find no thread and be skipped for
// good, so the backlog must drain oldest-first per merge request (SCEN-005).
func TestPostgres_RetryingDeliveryHoldsBackItsMergeRequest(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	later := now.Add(time.Hour + time.Second)

	otherOpen := newEvent("b-open")
	otherOpen.MRRef = "5678"
	enqueueAll(t, st, now, newEvent("a-open"), noteEvent("a-note", "create", false), otherOpen)

	root, id := claimDelivery(t, st, now)
	require.Equal(t, "a-open", id)
	require.NoError(t, st.Settle(ctx, root.ID, now,
		store.Retried(1, now.Add(time.Hour), "http 503")))

	other, id := claimDelivery(t, st, now)
	require.Equal(t, "b-open", id, "the comment must not overtake its deferred root")
	require.NoError(t, st.Settle(ctx, other.ID, now, store.Sent()))

	_, id = claimDelivery(t, st, now)
	assert.Empty(t, id, "nothing is claimable while the root waits")

	root, id = claimDelivery(t, st, later)
	require.Equal(t, "a-open", id, "the deferred root returns first once due")
	require.NoError(t, st.Settle(ctx, root.ID, later, store.Sent()))

	_, id = claimDelivery(t, st, later)
	assert.Equal(t, "a-note", id, "the comment follows its sent root")
}

func TestPostgres_ThreadLifecycle(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	th, err := st.Thread(ctx, "42")
	require.NoError(t, err)
	assert.Nil(t, th, "unknown MRs have no mapping (SYSREQ-007 input)")

	require.NoError(t, st.OpenThread(ctx, "42", "chan-1", "root-a", now))
	// re-opening is a no-op, not an error and not an overwrite
	require.NoError(t, st.OpenThread(ctx, "42", "chan-1", "root-b", now))

	th, err = st.Thread(ctx, "42")
	require.NoError(t, err)
	require.NotNil(t, th)
	assert.Equal(t, "root-a", th.RootPostID, "the first root wins")
	assert.True(t, th.Open)

	require.NoError(t, st.CloseThread(ctx, "42", now.Add(time.Minute)))
	th, err = st.Thread(ctx, "42")
	require.NoError(t, err)
	assert.False(t, th.Open, "closed mappings stay closed (SYSREQ-026)")
}

func TestPostgres_MarkOutcomeAndPrune(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, st.MarkOutcome(ctx, "old-msg", "expired", now.Add(-25*time.Hour)))
	require.NoError(t, st.MarkOutcome(ctx, "fresh-msg", "unrouted", now))

	seen, err := st.Seen(ctx, "old-msg")
	require.NoError(t, err)
	assert.True(t, seen, "discarded deliveries are seen-marked")

	require.NoError(t, st.PruneSeen(ctx, now.Add(-24*time.Hour)))

	seen, err = st.Seen(ctx, "old-msg")
	require.NoError(t, err)
	assert.False(t, seen, "rows past retention prune away")

	seen, err = st.Seen(ctx, "fresh-msg")
	require.NoError(t, err)
	assert.True(t, seen, "rows inside retention survive")

	require.NoError(t, st.Ping(ctx))
}

// guard against silent drift of the sqlc mapping: the payload round-trip must
// preserve every field the renderer relies on.
func TestPostgres_PayloadRoundTrip(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	ev := newEvent("msg-rt")
	ev.Title = "unicode ✓ & \"quotes\""
	res, err := st.Accept(ctx, ev, "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.True(t, res.Enqueued)

	job, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	got, err := job.Event()
	require.NoError(t, err)
	assert.Equal(t, ev, got, "jsonb round-trip must preserve every rendered field")
}
