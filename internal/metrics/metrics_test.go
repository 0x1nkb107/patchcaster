package metrics_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/metrics"
)

var scrapeTime = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func TestTheWaitingGaugesReportEverySinkQueue(t *testing.T) {
	// arrange
	m := metrics.New()
	m.RegisterQueues(func(context.Context) ([]metrics.SinkQueue, error) {
		return []metrics.SinkQueue{
			{ConnectionID: 2, SinkID: 5, Waiting: 3, OldestWaiting: scrapeTime.Add(-90 * time.Second)},
			{ConnectionID: 2, SinkID: 6},
		}, nil
	}, func() time.Time { return scrapeTime }, slog.New(slog.DiscardHandler))

	// act
	err := testutil.GatherAndCompare(m.Registry, strings.NewReader(`
# HELP patchcaster_sink_oldest_waiting_seconds Age of the oldest message waiting for a sink, 0 when none waits, by chat connection and sink.
# TYPE patchcaster_sink_oldest_waiting_seconds gauge
patchcaster_sink_oldest_waiting_seconds{connection="2",sink="5"} 90
patchcaster_sink_oldest_waiting_seconds{connection="2",sink="6"} 0
# HELP patchcaster_sink_waiting_messages Messages queued for a sink and not yet delivered, by chat connection and sink.
# TYPE patchcaster_sink_waiting_messages gauge
patchcaster_sink_waiting_messages{connection="2",sink="5"} 3
patchcaster_sink_waiting_messages{connection="2",sink="6"} 0
`), "patchcaster_sink_waiting_messages", "patchcaster_sink_oldest_waiting_seconds")

	// assert
	assert.NoError(t, err)
}

func TestAnUnreadableQueueFailsNoScrape(t *testing.T) {
	// arrange
	m := metrics.New()
	m.RegisterQueues(func(context.Context) ([]metrics.SinkQueue, error) {
		return nil, errors.New("database unavailable")
	}, func() time.Time { return scrapeTime }, slog.New(slog.DiscardHandler))
	m.CountReceived(metrics.ConnectionOf(1))

	// act
	families, err := m.Registry.Gather()

	// assert
	require.NoError(t, err)
	var names []string
	for _, family := range families {
		names = append(names, family.GetName())
	}
	assert.Contains(t, names, "patchcaster_deliveries_received_total")
	assert.NotContains(t, names, "patchcaster_sink_waiting_messages")
}

func TestLabelsNameConnectionsAndSinksByID(t *testing.T) {
	// arrange
	m := metrics.New()

	// act
	m.CountRejected(metrics.CauseEndpoint, metrics.NoConnection)
	m.CountRejected(metrics.CauseSignature, metrics.ConnectionOf(4))
	m.CountAttempt(metrics.OutcomeSent, metrics.SinkOf(2, 7), time.Second)

	// assert
	assert.Equal(t, 1.0, testutil.ToFloat64(m.Rejected.WithLabelValues(metrics.CauseEndpoint, "")))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.Rejected.WithLabelValues(metrics.CauseSignature, "4")))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.MMAttempts.WithLabelValues(metrics.OutcomeSent, "2", "7")))
	assert.Equal(t, 1, testutil.CollectAndCount(m.MMDuration), "the duration carries the connection only")
}
