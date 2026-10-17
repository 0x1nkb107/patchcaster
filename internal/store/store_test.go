package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// The PostgreSQL contract (ADR-010/ADR-006): these tests run against the
// real database that TestMain starts in a container, and recreate its
// public schema before every test (ADR-026).
func openTestStore(t *testing.T) (*store.Postgres, *sql.DB) {
	t.Helper()
	db := openEmptyDatabase(t)

	// Migrate through the production path (provider + session locker).
	applied, err := store.Migrate(context.Background(), db)
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

// The sink and chat connection every test delivery goes to. Queue rows name
// them without foreign keys, so they need no rows of their own.
const (
	testSinkID       = 1
	testConnectionID = 1
)

// toSinks delivers ev to the sinks with the given ids, with the plan ingest
// gave it.
func toSinks(ev *event.Event, sinkIDs ...int64) []store.Delivery {
	p := ev.Plan
	if p == nil {
		p = &event.Plan{}
	}
	deliveries := make([]store.Delivery, 0, len(sinkIDs))
	for _, id := range sinkIDs {
		deliveries = append(deliveries, store.Delivery{
			SinkID:       id,
			ConnectionID: testConnectionID,
			ChannelID:    "chan-1",
			Plan:         p,
		})
	}
	return deliveries
}

// acceptAtTestSink accepts ev for the test sink alone.
func acceptAtTestSink(ctx context.Context, st *store.Postgres, ev *event.Event, now time.Time) (store.AcceptResult, error) {
	return st.Accept(ctx, ev, toSinks(ev, testSinkID), now)
}

// enqueueAll accepts each delivery in order and requires a queue row for it.
func enqueueAll(t *testing.T, st *store.Postgres, at time.Time, events ...*event.Event) {
	t.Helper()
	for _, ev := range events {
		res, err := acceptAtTestSink(context.Background(), st, ev, at)
		require.NoError(t, err)
		require.Equal(t, 1, res.Enqueued, "delivery %q must enqueue", ev.WebhookID)
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
	res, err := acceptAtTestSink(ctx, st, newEvent("msg-1"), now)
	require.NoError(t, err)
	assert.True(t, res.Accepted)

	seen, err := st.Seen(ctx, "msg-1")
	require.NoError(t, err)
	assert.True(t, seen)

	// a replay of the same identifier is refused without a second enqueue
	res, err = acceptAtTestSink(ctx, st, newEvent("msg-1"), now)
	require.NoError(t, err)
	assert.False(t, res.Accepted, "the duplicate must not enqueue again")
	assert.Zero(t, res.Enqueued)

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
	_, err := acceptAtTestSink(ctx, st, newEvent("r-1"), now)
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
	_, err = acceptAtTestSink(ctx, st, newEvent("r-2"), now)
	require.NoError(t, err)
	job, err = st.ClaimDue(ctx, later, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, st.Settle(ctx, job.ID, later,
		store.Parked("attempt_budget", 8, "http 500")))

	// scenario 3: skipped is terminal without any Mattermost contact
	_, err = acceptAtTestSink(ctx, st, newEvent("r-3"), now)
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

// An in-flight delivery holds back the rest of its merge request, so a second
// worker never posts a reply beside a root that is still being posted.
func TestPostgres_ClaimedDeliveryHoldsBackItsMergeRequest(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	otherOpen := newEvent("b-open")
	otherOpen.MRRef = "5678"
	enqueueAll(t, st, now, newEvent("a-open"), noteEvent("a-note", "create", false), otherOpen)

	root, id := claimDelivery(t, st, now)
	require.Equal(t, "a-open", id)

	_, id = claimDelivery(t, st, now)
	require.Equal(t, "b-open", id, "the comment must wait for its claimed root")

	_, id = claimDelivery(t, st, now)
	assert.Empty(t, id, "nothing is claimable while both roots are in flight")

	require.NoError(t, st.Settle(ctx, root.ID, now, store.Sent()))
	_, id = claimDelivery(t, st, now)
	assert.Equal(t, "a-note", id, "the comment follows its sent root")
}

// A settled delivery releases its merge request whatever the outcome. A
// parked or skipped head must not wedge every later delivery behind it.
func TestPostgres_SettledDeliveryReleasesItsMergeRequest(t *testing.T) {
	tests := []struct {
		name       string
		settlement store.Settlement
	}{
		{name: "parked", settlement: store.Parked("attempt_budget", 8, "http 500")},
		{name: "skipped", settlement: store.Skipped("duplicate_open")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, _ := openTestStore(t)
			ctx := context.Background()
			now := time.Now().UTC().Truncate(time.Millisecond)

			enqueueAll(t, st, now, newEvent("a-open"), noteEvent("a-note", "create", false))

			root, id := claimDelivery(t, st, now)
			require.Equal(t, "a-open", id)
			require.NoError(t, st.Settle(ctx, root.ID, now, tc.settlement))

			_, id = claimDelivery(t, st, now)
			assert.Equal(t, "a-note", id, "a %s root must release its merge request", tc.name)
		})
	}
}

// newSink stores a sink to map threads to: thread mappings name their sink
// by foreign key.
func newSink(t *testing.T, st *store.Postgres, channel string) *store.Sink {
	t.Helper()
	chat := createConnection(t, st, "mattermost", "chat-"+channel, []byte("sealed"))
	return createSink(t, st, chat, "room "+channel, channel)
}

func TestPostgres_ThreadLifecycle(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	sink := newSink(t, st, "chan-1")

	th, err := st.Thread(ctx, "42", sink.ID)
	require.NoError(t, err)
	assert.Nil(t, th, "unknown MRs have no mapping (SYSREQ-007 input)")

	require.NoError(t, st.OpenThread(ctx, "42", sink.ID, "chan-1", "root-a", now))
	// re-opening is a no-op, not an error and not an overwrite
	require.NoError(t, st.OpenThread(ctx, "42", sink.ID, "chan-1", "root-b", now))

	th, err = st.Thread(ctx, "42", sink.ID)
	require.NoError(t, err)
	require.NotNil(t, th)
	assert.Equal(t, "root-a", th.RootPostID, "the first root wins")
	assert.True(t, th.Open)

	require.NoError(t, st.CloseThread(ctx, "42", sink.ID, now.Add(time.Minute)))
	th, err = st.Thread(ctx, "42", sink.ID)
	require.NoError(t, err)
	assert.False(t, th.Open, "closed mappings stay closed (SYSREQ-026)")
}

// Each sink keeps its own thread of a merge request (SYSREQ-048).
func TestPostgres_ThreadsArePerSink(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	first, second := newSink(t, st, "chan-a"), newSink(t, st, "chan-b")
	require.NoError(t, st.OpenThread(ctx, "42", first.ID, "chan-a", "root-a", now))
	require.NoError(t, st.OpenThread(ctx, "42", second.ID, "chan-b", "root-b", now))

	// act
	require.NoError(t, st.CloseThread(ctx, "42", first.ID, now))
	firstThread, _ := st.Thread(ctx, "42", first.ID)
	secondThread, _ := st.Thread(ctx, "42", second.ID)
	goneErr := st.OpenThread(ctx, "43", 9999, "chan-x", "root-x", now)

	// assert
	assert.False(t, firstThread.Open)
	assert.True(t, secondThread.Open, "closing one sink's thread leaves the other's")
	assert.Equal(t, "root-b", secondThread.RootPostID)
	assert.ErrorIs(t, goneErr, store.ErrNotFound, "a removed sink takes no thread")
}

// One sink's backlog holds back only that sink's messages (SYSREQ-048).
func TestPostgres_ClaimHoldsBackPerSink(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	open := newEvent("fan-open")
	_, err := st.Accept(ctx, open, toSinks(open, 1, 2), now)
	require.NoError(t, err)
	comment := noteEvent("fan-comment", "create", false)
	_, err = st.Accept(ctx, comment, toSinks(comment, 1, 2), now)
	require.NoError(t, err)

	// act: the first sink's root stays claimed, as if its platform hung.
	stuck, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	other, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	held, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)

	// assert
	require.NotNil(t, stuck)
	assert.Equal(t, int64(1), stuck.SinkID)
	require.NotNil(t, other)
	assert.Equal(t, int64(2), other.SinkID, "the other sink's root does not wait")
	assert.Nil(t, held, "both comments wait for their own sink's root")
}

// Each sink gets the plan its route shaped, and a sink whose plan moves
// nothing gets no message.
func TestPostgres_AcceptQueuesEachSinksPlan(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := acceptAtTestSink(ctx, st, mrEvent("plan-open", "open", nil), now)
	require.NoError(t, err)
	label := mrEvent("plan-label", "update", nil)
	muted := *label.Plan

	// act
	labelResult, err := st.Accept(ctx, label, []store.Delivery{
		{SinkID: 1, ConnectionID: testConnectionID, ChannelID: "chan-1", Plan: label.Plan},
		{SinkID: 2, ConnectionID: testConnectionID, ChannelID: "chan-2", Plan: &muted},
	}, now)
	require.NoError(t, err)
	comment := noteEvent("plan-comment", "create", false)
	silent := *comment.Plan
	silent.Notice = ""
	commentResult, commentErr := st.Accept(ctx, comment, []store.Delivery{
		{SinkID: 1, ConnectionID: testConnectionID, ChannelID: "chan-1", Plan: comment.Plan},
		{SinkID: 2, ConnectionID: testConnectionID, ChannelID: "chan-2", Plan: &silent},
	}, now)

	// assert
	assert.Zero(t, labelResult.Enqueued, "a label-only update moves nothing in either sink")
	require.NoError(t, commentErr)
	assert.Equal(t, 2, commentResult.Enqueued, "the muted sink still refreshes its board")
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
	res, err := acceptAtTestSink(ctx, st, ev, now)
	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Equal(t, 1, res.Enqueued)

	job, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	got, err := job.Event()
	require.NoError(t, err)
	want := *ev
	want.Plan = &event.Plan{CreateRoot: true, Refresh: true} // the sink's plan, finalized at acceptance
	assert.Equal(t, &want, got, "jsonb round-trip must preserve every rendered field")
}
