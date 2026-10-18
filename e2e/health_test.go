package e2e

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// metricValue reads one series of the service's metrics, the one of name
// with exactly labels. It reports false when the series is missing.
// Prometheus writes label pairs ordered by name, and so does the series
// this looks for.
func metricValue(t *testing.T, svc *service, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	resp, err := http.Get(svc.internalURL + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	names := make([]string, 0, len(labels))
	for label := range labels {
		names = append(names, label)
	}
	slices.Sort(names)
	pairs := make([]string, 0, len(names))
	for _, label := range names {
		pairs = append(pairs, label+"="+strconv.Quote(labels[label]))
	}
	series := name + "{" + strings.Join(pairs, ",") + "} "
	for line := range strings.Lines(string(body)) {
		value, found := strings.CutPrefix(strings.TrimSuffix(line, "\n"), series)
		if !found {
			continue
		}
		v, err := strconv.ParseFloat(value, 64)
		require.NoError(t, err)
		return v, true
	}
	return 0, false
}

// sinkLabels names a sink in the metrics.
func sinkLabels(sink *apiv1.Sink, extra ...string) map[string]string {
	labels := map[string]string{
		"connection": strconv.FormatInt(sink.ConnectionID, 10),
		"sink":       strconv.FormatInt(sink.ID, 10),
	}
	for i := 0; i+1 < len(extra); i += 2 {
		labels[extra[i]] = extra[i+1]
	}
	return labels
}

// A sink that cannot deliver shows in its metrics and in the health the
// API reports, while the healthy sink and readiness stay as they were
// (SYSREQ-064).
func TestAFailingSinkShowsInItsHealth(t *testing.T) {
	// arrange
	healthy, failing := newFakeMattermost(t), newFakeMattermost(t)
	freshDatabase(t)
	svc, r, _ := startRouted(t, serviceConfig{}, healthy)
	second := connectChat(t, r.admin, "second mattermost", failing)
	broken := r.addSink(t, second, "second channel", channelID)
	r.addRoute(t, broken, nil)
	failing.setFailing(true)
	admin := r.admin.api(t)
	ctx := context.Background()

	// act
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	eventually(t, 10*time.Second, "the healthy root and a failed attempt in the metrics", func() bool {
		failed, _ := metricValue(t, svc, "patchcaster_mattermost_attempts_total", sinkLabels(broken, "outcome", "retryable"))
		return len(healthy.acceptedPosts()) == 1 && failed >= 1
	})

	// assert: the metrics tell the sinks apart.
	sent, _ := metricValue(t, svc, "patchcaster_messages_posted_total", sinkLabels(r.sink))
	assert.Equal(t, 1.0, sent, "the healthy sink posted")
	brokenWaiting, found := metricValue(t, svc, "patchcaster_sink_waiting_messages", sinkLabels(broken))
	require.True(t, found, "every sink has a waiting gauge")
	assert.Equal(t, 1.0, brokenWaiting, "the failing sink's message waits")
	healthyWaiting, found := metricValue(t, svc, "patchcaster_sink_waiting_messages", sinkLabels(r.sink))
	require.True(t, found)
	assert.Equal(t, 0.0, healthyWaiting)
	received, _ := metricValue(t, svc, "patchcaster_deliveries_received_total",
		map[string]string{"connection": strconv.FormatInt(r.forge.ID, 10)})
	assert.Equal(t, 1.0, received, "the forge connection's delivery counts under its id")

	// assert: the API reports the same health for the interface.
	brokenRead, err := admin.GetSink(ctx, apiv1.GetSinkParams{ID: broken.ID})
	require.NoError(t, err)
	brokenHealth := brokenRead.Response.Health
	assert.Equal(t, int64(1), brokenHealth.WaitingMessages)
	assert.True(t, brokenHealth.OldestWaitingSince.IsSet())
	assert.True(t, brokenHealth.LastFailureAt.IsSet())
	assert.Contains(t, brokenHealth.LastError.Or(""), "503", "the error names the platform's answer")
	assert.False(t, brokenHealth.LastSuccessAt.IsSet(), "the failing sink never delivered")

	healthyRead, err := admin.GetSink(ctx, apiv1.GetSinkParams{ID: r.sink.ID})
	require.NoError(t, err)
	assert.True(t, healthyRead.Response.Health.LastSuccessAt.IsSet())
	assert.False(t, healthyRead.Response.Health.LastFailureAt.IsSet())

	forgeRead, err := admin.GetConnection(ctx, apiv1.GetConnectionParams{ID: r.forge.ID})
	require.NoError(t, err)
	assert.True(t, forgeRead.Response.Health.LastDeliveryAt.IsSet(), "the forge connection's delivery arrived")
	secondRead, err := admin.GetConnection(ctx, apiv1.GetConnectionParams{ID: second.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), secondRead.Response.Health.WaitingMessages.Or(0))

	// assert: one sink's trouble leaves the instance in rotation.
	assert.Equal(t, http.StatusOK, svc.readiness())
}

// A delivery with a wrong signature shows in its forge connection's health
// (SYSREQ-064).
func TestARejectedDeliveryShowsInItsConnectionsHealth(t *testing.T) {
	// arrange
	mattermost := newFakeMattermost(t)
	freshDatabase(t)
	svc, r, _ := startRouted(t, serviceConfig{}, mattermost)

	// act
	status := svc.gitlab.deliverSigned(t, newDeliveryID(t), mergeRequestOpened(t, firstMR), "not-the-secret")

	// assert
	require.Equal(t, http.StatusUnauthorized, status)
	read, err := r.admin.api(t).GetConnection(context.Background(), apiv1.GetConnectionParams{ID: r.forge.ID})
	require.NoError(t, err)
	health := read.Response.Health
	assert.True(t, health.LastRejectionAt.IsSet())
	assert.Equal(t, apiv1.ConnectionHealthLastRejectionSignature, health.LastRejection.Or(""))
	assert.False(t, health.LastDeliveryAt.IsSet(), "a forged delivery is no delivery of the connection")
	rejected, _ := metricValue(t, svc, "patchcaster_deliveries_rejected_total",
		map[string]string{"cause": "signature", "connection": strconv.FormatInt(r.forge.ID, 10)})
	assert.Equal(t, 1.0, rejected)
}
