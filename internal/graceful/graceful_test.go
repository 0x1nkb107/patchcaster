package graceful_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/graceful"
)

// server is one Serve call on a loopback listener.
type server struct {
	ln     net.Listener
	addr   string
	cancel context.CancelFunc
	done   chan error
}

func startServe(t *testing.T, handler http.Handler, budget time.Duration) *server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &server{ln: ln, addr: ln.Addr().String(), cancel: cancel, done: make(chan error, 1)}
	go func() { s.done <- graceful.Serve(ctx, ln, handler, budget) }()
	return s
}

// awaitEnd returns Serve's result, failing the test on a hang.
func (s *server) awaitEnd(t *testing.T) error {
	t.Helper()
	select {
	case err := <-s.done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return")
		return nil
	}
}

// awaitClosedListener waits until new connections are refused, which is the
// first thing a shutdown does.
func (s *server) awaitClosedListener(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", s.addr)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the listener never closed")
}

// get requests path in the background and reports the status code, or 0 when
// the request failed.
func get(addr string) <-chan int {
	status := make(chan int, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://" + addr + "/")
		if err != nil {
			status <- 0
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		status <- resp.StatusCode
	}()
	return status
}

// heldHandler answers only once released, reporting each request it holds.
func heldHandler() (http.Handler, <-chan struct{}, chan struct{}) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	return h, entered, release
}

func TestServe_StopsWhenItsContextEnds(t *testing.T) {
	// arrange
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	s := startServe(t, ok, time.Second)
	require.Equal(t, http.StatusOK, <-get(s.addr))

	// act
	s.cancel()

	// assert
	assert.ErrorIs(t, s.awaitEnd(t), graceful.ErrStopped)
}

func TestServe_LetsAnInFlightRequestFinish(t *testing.T) {
	// arrange: a request the handler holds until the test releases it.
	handler, entered, release := heldHandler()
	s := startServe(t, handler, 2*time.Second)
	status := get(s.addr)
	<-entered

	// act: shut down, and release the request only once the listener has
	// closed, so the response can only arrive through the drain.
	s.cancel()
	s.awaitClosedListener(t)
	close(release)

	// assert
	assert.Equal(t, http.StatusNoContent, <-status, "the request in flight completes")
	assert.ErrorIs(t, s.awaitEnd(t), graceful.ErrStopped)
}

func TestServe_CutsOffRequestsBeyondTheBudget(t *testing.T) {
	// arrange: a request that outlives the shutdown budget. The handler is
	// released only when the test ends, so a response before that can only
	// mean the connection was cut.
	handler, entered, release := heldHandler()
	t.Cleanup(func() { close(release) })
	s := startServe(t, handler, 20*time.Millisecond)
	status := get(s.addr)
	<-entered

	// act
	s.cancel()

	// assert: the expired budget is reported, and the held request fails.
	err := s.awaitEnd(t)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorContains(t, err, "shut down "+s.addr)
	select {
	case code := <-status:
		assert.Zero(t, code, "the held request must fail, not answer late")
	case <-time.After(2 * time.Second):
		t.Fatal("the held connection was left open")
	}
}

func TestServe_ReportsAFailedListenerAtOnce(t *testing.T) {
	// arrange
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	s := startServe(t, ok, time.Second)
	require.Equal(t, http.StatusOK, <-get(s.addr))

	// act: the listener breaks while the context is still live.
	require.NoError(t, s.ln.Close())

	// assert: Serve returns the failure without waiting for shutdown.
	err := s.awaitEnd(t)
	assert.NotErrorIs(t, err, graceful.ErrStopped)
	assert.ErrorContains(t, err, "serve "+s.addr)
	assert.ErrorIs(t, err, net.ErrClosed)
}
