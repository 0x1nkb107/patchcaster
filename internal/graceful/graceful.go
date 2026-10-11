// Package graceful runs HTTP servers that stop with their context: the
// listener closes at once, and requests already in flight get a budget to
// finish. The public webhook listener and the internal probe listener both
// run on it, so they shut down the same way (SYSREQ-023).
package graceful

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ErrStopped reports that a server ended through an orderly shutdown.
var ErrStopped = errors.New("listener stopped")

// Serve serves ln with handler until ctx ends, then drains in-flight requests
// within the shutdown budget. It returns ErrStopped after an orderly
// shutdown. A server that fails before ctx ends returns its error at once,
// and requests still open when the budget runs out are cut off. Errors name
// the listener's address, because several listeners share this function.
// Callers bind the socket first so a bind failure surfaces before the
// service claims readiness.
func Serve(ctx context.Context, ln net.Listener, handler http.Handler, shutdown time.Duration) error {
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	// Wait for cancellation or an unexpected server exit.
	select {
	case err := <-errCh:
		// The listener is gone; end the connections it left behind.
		_ = srv.Close()
		return fmt.Errorf("serve %s: %w", ln.Addr(), err)
	case <-ctx.Done():
	}

	// ctx has ended, so the drain needs a fresh budget of its own.
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdown)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil {
		// Cut off the connections the budget could not drain.
		_ = srv.Close()
		return fmt.Errorf("shut down %s: %w", ln.Addr(), err)
	}

	// After a clean shutdown Serve reports ErrServerClosed; anything else
	// is a failure that coincided with the shutdown.
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve %s: %w", ln.Addr(), err)
	}
	return ErrStopped
}
