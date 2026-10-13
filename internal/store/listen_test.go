package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Wake-up contract (ADR-019): an enqueue notifies the outbox channel on
// commit, and a listening session turns that into wake-ups.

const outboxChannel = "patchcaster_outbox"

// listenRaw subscribes a plain connection to the outbox channel, so a test
// can observe what ingest sends without going through OutboxListener.
func listenRaw(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	_, err = conn.Exec(ctx, "LISTEN "+outboxChannel)
	require.NoError(t, err)
	return conn
}

// countNotificationsBeforeSentinel sends a sentinel through the channel and
// counts what arrives ahead of it. PostgreSQL delivers notifications in
// commit order, so the count is exact without waiting on a clock.
func countNotificationsBeforeSentinel(t *testing.T, conn *pgx.Conn) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := conn.Exec(ctx, "SELECT pg_notify($1, 'sentinel')", outboxChannel)
	require.NoError(t, err)
	count := 0
	for {
		n, err := conn.WaitForNotification(ctx)
		require.NoError(t, err, "the sentinel must arrive")
		if n.Payload == "sentinel" {
			return count
		}
		count++
	}
}

// startListener runs one OutboxListener session in the background and
// reports each wake-up and the session's final error.
func startListener(t *testing.T, ctx context.Context) (<-chan struct{}, <-chan error) {
	t.Helper()
	listener, err := store.NewOutboxListener(databaseURL)
	require.NoError(t, err)

	wakes := make(chan struct{}, 8)
	done := make(chan error, 1)
	go func() {
		done <- listener.Listen(ctx, func() { wakes <- struct{}{} })
	}()
	return wakes, done
}

// awaitWake fails the test unless a wake-up arrives. The timeout only guards
// against a hang; a healthy session answers in milliseconds.
func awaitWake(t *testing.T, wakes <-chan struct{}, why string) {
	t.Helper()
	select {
	case <-wakes:
	case <-time.After(5 * time.Second):
		t.Fatalf("no wake-up: %s", why)
	}
}

// awaitEnd returns the session's final error, failing the test on a hang.
func awaitEnd(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the listening session did not end")
		return nil
	}
}

func TestPostgres_AcceptNotifiesForEnqueuedRowsOnly(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	observer := listenRaw(t)

	res, err := st.Accept(ctx, mrEvent("n-open", "open", nil), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Enqueued)

	res, err = st.Accept(ctx, mrEvent("n-open", "open", nil), "chan-1", now)
	require.NoError(t, err)
	require.False(t, res.Accepted, "a duplicate commits nothing")

	res, err = st.Accept(ctx, mrEvent("n-label", "update", nil), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.False(t, res.Enqueued, "a label-only update enqueues no row")

	assert.Equal(t, 1, countNotificationsBeforeSentinel(t, observer),
		"only the delivery that enqueued a row may notify")
}

func TestOutboxListener_WakesOnceListeningAndPerRow(t *testing.T) {
	st, _ := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wakes, done := startListener(t, ctx)
	awaitWake(t, wakes, "a confirmed session wakes the worker to catch up")

	_, err := st.Accept(context.Background(), newEvent("w-1"), "chan-1", time.Now().UTC())
	require.NoError(t, err)
	awaitWake(t, wakes, "a committed row wakes the worker")

	cancel()
	assert.ErrorIs(t, awaitEnd(t, done), context.Canceled)
	assert.Empty(t, wakes, "the probe and the row woke the worker once each")
}

func TestOutboxListener_EndsWhenItsConnectionDies(t *testing.T) {
	_, db := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wakes, done := startListener(t, ctx)
	awaitWake(t, wakes, "a confirmed session wakes the worker to catch up")

	// The server drops every other session of this role, the listener's
	// among them, as a failover or an administrator would.
	_, err := db.ExecContext(context.Background(), `
		SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE datname = current_database() AND usename = current_user
		  AND backend_type = 'client backend' AND pid <> pg_backend_pid()`)
	require.NoError(t, err)

	err = awaitEnd(t, done)
	require.Error(t, err, "a dead connection ends the session")
	assert.NoError(t, ctx.Err(), "the session ended on its own, not by cancellation")
	assert.NotErrorIs(t, err, context.Canceled)
}
