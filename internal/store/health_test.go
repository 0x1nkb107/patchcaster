package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// healthClock is a fixed instant the health tests count from. PostgreSQL
// keeps microseconds, so it has none finer.
var healthClock = time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

func at(minutes int) time.Time {
	return healthClock.Add(time.Duration(minutes) * time.Minute)
}

// requireSameInstant asserts that got is the instant want, whatever its
// location.
func requireSameInstant(t *testing.T, want, got time.Time, what string) {
	t.Helper()
	assert.Truef(t, want.Equal(got), "%s: want %s, got %s", what, want, got)
}

func TestASinkKeepsItsLatestOutcomes(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	sink := newSink(t, st, "c-1")

	// act: outcomes arrive out of order, as from two replicas.
	require.NoError(t, st.RecordSinkSuccess(ctx, sink.ID, at(2)))
	require.NoError(t, st.RecordSinkFailure(ctx, sink.ID, at(3), "post: 500 Internal Server Error"))
	require.NoError(t, st.RecordSinkSuccess(ctx, sink.ID, at(1)))
	require.NoError(t, st.RecordSinkFailure(ctx, sink.ID, at(0), "an older failure"))
	health, err := st.SinkHealth(ctx, []int64{sink.ID})

	// assert
	require.NoError(t, err)
	h := health[sink.ID]
	requireSameInstant(t, at(2), h.LastSuccess, "last success")
	requireSameInstant(t, at(3), h.LastFailure, "last failure")
	assert.Equal(t, "post: 500 Internal Server Error", h.LastError, "a late, older failure leaves the newer one")
}

func TestASinkWithoutDeliveriesHasZeroHealth(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	sink := newSink(t, st, "c-1")

	// act
	health, err := st.SinkHealth(context.Background(), []int64{sink.ID, 9999})

	// assert
	require.NoError(t, err)
	assert.Equal(t, store.SinkHealth{}, health[sink.ID])
	assert.NotContains(t, health, int64(9999), "a sink that does not exist has no health")
}

func TestALongErrorIsCutShort(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	sink := newSink(t, st, "c-1")

	// act
	require.NoError(t, st.RecordSinkFailure(ctx, sink.ID, at(0), strings.Repeat("é", 5000)))
	health, err := st.SinkHealth(ctx, []int64{sink.ID})

	// assert
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("é", 1000), health[sink.ID].LastError)
}

func TestWaitingMessagesCountUntilTheyAreSettled(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	busy, idle := newSink(t, st, "c-busy"), newSink(t, st, "c-idle")
	for i, id := range []string{"w-1", "w-2", "w-3"} {
		ev := newEvent(id)
		ev.MRRef = "1:" + id
		_, err := st.Accept(ctx, ev, toSinks(ev, busy.ID), at(i))
		require.NoError(t, err)
	}
	job, _ := claimDelivery(t, st, at(10))
	require.NotNil(t, job)
	require.NoError(t, st.Settle(ctx, job.ID, at(10), store.Sent()))
	_, _ = claimDelivery(t, st, at(10))

	// act
	health, healthErr := st.SinkHealth(ctx, []int64{busy.ID, idle.ID})
	queues, queuesErr := st.SinkQueues(ctx)

	// assert
	require.NoError(t, healthErr)
	assert.Equal(t, int64(2), health[busy.ID].Waiting, "a claimed message still waits")
	requireSameInstant(t, at(1), health[busy.ID].OldestWaiting, "oldest waiting")
	assert.Equal(t, store.SinkHealth{}, health[idle.ID])
	require.NoError(t, queuesErr)
	require.Len(t, queues, 2, "every sink has a queue, an empty one too")
	assert.Equal(t, busy.ID, queues[0].SinkID)
	assert.Equal(t, busy.ConnectionID, queues[0].ConnectionID)
	assert.Equal(t, int64(2), queues[0].Waiting)
	requireSameInstant(t, at(1), queues[0].OldestWaiting, "oldest in the queue")
	assert.Equal(t, int64(0), queues[1].Waiting)
	assert.True(t, queues[1].OldestWaiting.IsZero())
}

func TestAChatConnectionSumsUpItsSinks(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	chat := createConnection(t, st, "mattermost", "chat", []byte("sealed"))
	lonely := createConnection(t, st, "mattermost", "lonely", []byte("sealed"))
	first, second := createSink(t, st, chat, "first", "c-1"), createSink(t, st, chat, "second", "c-2")
	require.NoError(t, st.RecordSinkSuccess(ctx, first.ID, at(5)))
	require.NoError(t, st.RecordSinkFailure(ctx, first.ID, at(1), "first failed"))
	require.NoError(t, st.RecordSinkSuccess(ctx, second.ID, at(2)))
	require.NoError(t, st.RecordSinkFailure(ctx, second.ID, at(4), "second failed"))
	ev := newEvent("w-1")
	_, err := st.Accept(ctx, ev, toSinks(ev, first.ID, second.ID), at(3))
	require.NoError(t, err)

	// act
	health, err := st.ChatConnectionHealth(ctx, []int64{chat.ID, lonely.ID})

	// assert
	require.NoError(t, err)
	h := health[chat.ID]
	assert.Equal(t, int64(2), h.Waiting)
	requireSameInstant(t, at(3), h.OldestWaiting, "oldest waiting")
	requireSameInstant(t, at(5), h.LastSuccess, "latest success of any sink")
	requireSameInstant(t, at(4), h.LastFailure, "latest failure of any sink")
	assert.Equal(t, "second failed", h.LastError, "the error of the latest failure")
	assert.Equal(t, store.SinkHealth{}, health[lonely.ID], "a connection without sinks has zero health")
}

func TestAForgeConnectionKeepsItsLatestDeliveryAndRejection(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", nil)
	quiet := createConnection(t, st, "gitlab", "quiet", nil)

	// act
	require.NoError(t, st.RecordForgeDelivery(ctx, forge.ID, at(3)))
	require.NoError(t, st.RecordForgeDelivery(ctx, forge.ID, at(1)))
	require.NoError(t, st.RecordForgeRejection(ctx, forge.ID, at(2), "signature"))
	require.NoError(t, st.RecordForgeRejection(ctx, forge.ID, at(0), "parse"))
	health, err := st.ForgeHealth(ctx, []int64{forge.ID, quiet.ID})

	// assert
	require.NoError(t, err)
	h := health[forge.ID]
	requireSameInstant(t, at(3), h.LastDelivery, "last delivery")
	requireSameInstant(t, at(2), h.LastRejection, "last rejection")
	assert.Equal(t, "signature", h.RejectionCause)
	assert.Equal(t, store.ForgeHealth{}, health[quiet.ID])
}

func TestHealthGoesWithItsSinkOrConnection(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	chat := createConnection(t, st, "mattermost", "chat", []byte("sealed"))
	forge := createConnection(t, st, "gitlab", "gitlab", nil)
	sink := createSink(t, st, chat, "room", "c-1")
	require.NoError(t, st.RecordSinkSuccess(ctx, sink.ID, at(0)))
	require.NoError(t, st.RecordForgeDelivery(ctx, forge.ID, at(0)))

	// act
	require.NoError(t, st.DeleteSink(ctx, routingActor, sink.ID, sink.Version, at(1)))
	require.NoError(t, st.DeleteConnection(ctx, routingActor, forge.ID, forge.Version, at(1)))
	lateSink := st.RecordSinkFailure(ctx, sink.ID, at(2), "too late")
	lateForge := st.RecordForgeRejection(ctx, forge.ID, at(2), "signature")
	sinks, sinksErr := st.SinkHealth(ctx, []int64{sink.ID})
	forges, forgesErr := st.ForgeHealth(ctx, []int64{forge.ID})

	// assert
	require.NoError(t, lateSink, "a removed sink records nothing, and that is no error")
	require.NoError(t, lateForge, "a removed connection records nothing, and that is no error")
	require.NoError(t, sinksErr)
	assert.Empty(t, sinks)
	require.NoError(t, forgesErr)
	assert.Equal(t, store.ForgeHealth{}, forges[forge.ID])
}
