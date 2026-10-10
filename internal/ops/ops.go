// Package ops serves the internal listener: liveness, readiness, and
// Prometheus metrics (SYSARCH-004). It never depends on Mattermost health —
// Mattermost outages must not restart the pod via liveness.
package ops

import (
	"context"
	"errors"
	"net"
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

// Server owns the internal listener and its readiness bit.
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

// ErrStopped distinguishes normal http.Server.Shutdown termination.
var ErrStopped = errors.New("listener stopped")

// ListenAndServe serves the given listener until ctx ends, then drains
// in-flight requests within the shutdown budget. Callers bind the socket
// first so a bind failure surfaces before the service claims readiness.
func ListenAndServe(ctx context.Context, ln net.Listener, handler http.Handler, shutdown time.Duration) error {
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	<-ctx.Done()
	drain, cancel := context.WithTimeout(context.Background(), shutdown)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil {
		return err
	}
	if err := <-errCh; errors.Is(err, http.ErrServerClosed) {
		return ErrStopped
	}
	return nil
}
