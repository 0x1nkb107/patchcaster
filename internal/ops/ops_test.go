package ops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/metrics"
)

type stubPinger struct{ err error }

func (s stubPinger) Ping(context.Context) error { return s.err }

func probe(t *testing.T, srv *Server, path string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}

func TestLivenessNeverFails(t *testing.T) {
	// arrange: even a dead store must not trip liveness.
	srv := NewServer(stubPinger{err: errors.New("db gone")}, metrics.New())

	// act & assert
	assert.Equal(t, http.StatusOK, probe(t, srv, "/livez"))
}

func TestReadinessSequence(t *testing.T) {
	// arrange
	srv := NewServer(stubPinger{}, metrics.New())

	// assert: not ready before MarkReady.
	assert.Equal(t, http.StatusServiceUnavailable, probe(t, srv, "/readyz"), "start-up gate")

	srv.MarkReady(true)
	assert.Equal(t, http.StatusOK, probe(t, srv, "/readyz"))

	srv.MarkReady(false)
	assert.Equal(t, http.StatusServiceUnavailable, probe(t, srv, "/readyz"), "drain flips readiness")
}

func TestReadinessTracksStore(t *testing.T) {
	// arrange
	srv := NewServer(stubPinger{}, metrics.New())
	srv.MarkReady(true)

	// act & assert
	srv.pinger = stubPinger{err: errors.New("connection refused")}
	assert.Equal(t, http.StatusServiceUnavailable, probe(t, srv, "/readyz"))
}

func TestMetricsEndpointExposesFunnel(t *testing.T) {
	// arrange
	m := metrics.New()
	m.Received.Inc()
	srv := NewServer(stubPinger{}, m)

	// act
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	// assert
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "patchcaster_deliveries_received_total 1")
}
