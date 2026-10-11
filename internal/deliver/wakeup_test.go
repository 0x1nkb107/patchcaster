package deliver

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/metrics"
)

// session scripts one listening session of fakeListener.
type session func(ctx context.Context, wake func()) error

// fakeListener runs one scripted session per Listen call. Once the scripts
// run out it listens quietly until ctx ends.
type fakeListener struct {
	sessions []session
	started  chan int // receives each session's 1-based number as it starts
	calls    int
}

func (f *fakeListener) Listen(ctx context.Context, wake func()) error {
	f.calls++
	f.started <- f.calls
	if len(f.sessions) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}
	next := f.sessions[0]
	f.sessions = f.sessions[1:]
	return next(ctx, wake)
}

type wakeupHarness struct {
	wakeup   *Wakeup
	listener *fakeListener
	m        *metrics.Metrics
	cancel   context.CancelFunc
	done     chan error
}

// startWakeup runs a Wakeup over the scripted sessions, reporting to m.
// Re-listening waits no time, so the tests never depend on the clock.
func startWakeup(t *testing.T, m *metrics.Metrics, sessions ...session) *wakeupHarness {
	t.Helper()
	h := &wakeupHarness{
		listener: &fakeListener{sessions: sessions, started: make(chan int, 8)},
		m:        m,
		done:     make(chan error, 1),
	}
	h.wakeup = NewWakeup(h.listener, h.m, slog.New(slog.DiscardHandler))
	h.wakeup.relisten = &backoff.ZeroBackOff{}

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	t.Cleanup(cancel)
	go func() { h.done <- h.wakeup.Run(ctx) }()
	return h
}

// awaitSession waits until the given session has started.
func (h *wakeupHarness) awaitSession(t *testing.T, want int) {
	t.Helper()
	for {
		select {
		case got := <-h.listener.started:
			if got == want {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("session %d never started", want)
		}
	}
}

// stop ends Run and requires the clean nil return.
func (h *wakeupHarness) stop(t *testing.T) {
	t.Helper()
	h.cancel()
	select {
	case err := <-h.done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// listenUntilCancelled is the tail of a healthy session.
func listenUntilCancelled(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestWakeup_ConfirmedSessionWakesAndRaisesGauge(t *testing.T) {
	// arrange: the confirming wake-up, then two committed rows.
	notified := make(chan struct{})
	h := startWakeup(t, metrics.New(), func(ctx context.Context, wake func()) error {
		wake()
		wake()
		wake()
		close(notified)
		return listenUntilCancelled(ctx)
	})

	// act
	<-notified

	// assert: three signals coalesce into one pending wake-up.
	assert.Len(t, h.wakeup.wake, 1)
	assert.Equal(t, 1.0, testutil.ToFloat64(h.m.ListenerUp))

	h.stop(t)
	assert.Equal(t, 0.0, testutil.ToFloat64(h.m.ListenerUp), "a session ended by shutdown lowers the gauge")
}

func TestWakeup_RelistensAfterLostSession(t *testing.T) {
	// arrange: a confirmed session that drops, then one that holds. The
	// second session records the gauge before it confirms.
	m := metrics.New()
	gaugeBetween := make(chan float64, 1)
	confirmed := make(chan struct{})
	h := startWakeup(t, m,
		func(_ context.Context, wake func()) error {
			wake()
			return errors.New("connection reset by peer")
		},
		func(ctx context.Context, wake func()) error {
			gaugeBetween <- testutil.ToFloat64(m.ListenerUp)
			wake()
			close(confirmed)
			return listenUntilCancelled(ctx)
		},
	)

	// act
	<-confirmed

	// assert
	assert.Equal(t, 0.0, <-gaugeBetween, "the lost session lowered the gauge")
	assert.Equal(t, 1.0, testutil.ToFloat64(h.m.ListenerUp), "the new session raised it again")
	assert.Len(t, h.wakeup.wake, 1, "each confirmed session wakes the worker to catch up")
	h.stop(t)
}

func TestWakeup_FailedSessionNeitherWakesNorRaisesGauge(t *testing.T) {
	// arrange: a session that never gets as far as listening, for example
	// a refused connection or a probe that never came back.
	h := startWakeup(t, metrics.New(), func(context.Context, func()) error {
		return errors.New("connect outbox listener: connection refused")
	})

	// act: a second session starts only after the first one's failure was
	// handled, so waiting for it is waiting for that handling.
	h.awaitSession(t, 2)

	// assert
	assert.Empty(t, h.wakeup.wake, "an unconfirmed session must not wake the worker")
	assert.Equal(t, 0.0, testutil.ToFloat64(h.m.ListenerUp))
	h.stop(t)
}
