// Package graceful runs HTTP servers that stop with their context: the
// listener closes at once, and requests already in flight get a budget to
// finish. The public webhook listener and the internal probe listener both
// run on it, so they shut down the same way (SYSREQ-023).
package graceful

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// ErrStopped reports that a server ended through an orderly shutdown.
var ErrStopped = errors.New("listener stopped")

// Serve serves ln with handler until ctx ends, then drains in-flight requests
// within the shutdown budget. It returns ErrStopped after an orderly
// shutdown, or the shutdown error when the budget runs out. Callers bind the
// socket first so a bind failure surfaces before the service claims
// readiness.
func Serve(ctx context.Context, ln net.Listener, handler http.Handler, shutdown time.Duration) error {
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
