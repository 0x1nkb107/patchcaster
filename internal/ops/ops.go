// Package ops provides the internal listener's handlers: liveness,
// readiness, and Prometheus metrics (SYSARCH-004). It never depends on
// Mattermost health — Mattermost outages must not restart the pod via
// liveness. Package graceful serves the handlers.
package ops

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/patchcaster-app/patchcaster/internal/metrics"
)

// Pinger reports store availability for the readiness probe.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server holds the internal listener's handlers and its readiness bit.
type Server struct {
	pinger  Pinger
	metrics *metrics.Metrics
	ready   atomic.Bool
}

// NewServer starts with readiness false until MarkReady (after wiring).
func NewServer(pinger Pinger, m *metrics.Metrics) *Server {
	return &Server{pinger: pinger, metrics: m}
}

// MarkReady flips readiness on once the service can durably accept work.
func (s *Server) MarkReady(on bool) { s.ready.Store(on) }

// Mux exposes /livez /readyz /metrics for the internal listener.
func (s *Server) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !s.ready.Load() {
			http.Error(w, "draining or starting", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.pinger.Ping(ctx); err != nil {
			http.Error(w, "store unreachable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(
		s.metrics.Registry, promhttp.HandlerOpts{Registry: s.metrics.Registry}))
	return mux
}
