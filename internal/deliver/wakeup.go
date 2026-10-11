package deliver

import (
	"context"
	"log/slog"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/patchcaster-app/patchcaster/internal/metrics"
)

// Re-listen pacing after a lost session: quick at first, so a database
// restart costs little, and capped, so a long outage costs one connection
// attempt a minute. Jitter keeps replicas that lost the same server from
// reconnecting in lockstep.
const (
	relistenInitial    = time.Second
	relistenMultiplier = 2.0
	relistenMax        = time.Minute
	relistenJitter     = 0.5
)

// Listener holds listening sessions on committed outbox rows
// (store.OutboxListener). Listen calls wake from the calling goroutine, once
// as soon as the session listens and once per committed row after that, and
// returns when the session ends.
type Listener interface {
	Listen(ctx context.Context, wake func()) error
}

// Wakeup turns outbox notifications into worker wake-ups (ADR-019). While no
// session listens the worker still delivers, only at the poll interval;
// Wakeup listens again after backoff and reports the session state as a
// gauge, so the slower mode is visible.
type Wakeup struct {
	listener Listener
	metrics  *metrics.Metrics
	log      *slog.Logger
	wake     chan struct{}
	relisten backoff.BackOff
}

// NewWakeup wires a wake-up source over a listener.
func NewWakeup(l Listener, m *metrics.Metrics, log *slog.Logger) *Wakeup {
	relisten := &backoff.ExponentialBackOff{
		InitialInterval:     relistenInitial,
		RandomizationFactor: relistenJitter,
		Multiplier:          relistenMultiplier,
		MaxInterval:         relistenMax,
	}
	relisten.Reset()
	return &Wakeup{
		listener: l,
		metrics:  m,
		log:      log,
		wake:     make(chan struct{}, 1),
		relisten: relisten,
	}
}

// C delivers the wake-ups; pass it as Config.Wake. Wake-ups coalesce:
// however many rows commit while the worker is busy, one stays pending.
func (w *Wakeup) C() <-chan struct{} { return w.wake }

// Run keeps a session listening until ctx ends, then returns nil. A lost or
// failed session is logged and replaced after backoff.
func (w *Wakeup) Run(ctx context.Context) error {
	for {
		err := w.listen(ctx)
		if ctx.Err() != nil {
			return nil
		}

		delay := w.relisten.NextBackOff()
		w.log.Warn("outbox listener down; polling until it listens again",
			"err", err, "retry_in", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// listen holds one session. Its first wake-up confirms that the session
// listens, so that is when the gauge rises and the backoff starts over.
func (w *Wakeup) listen(ctx context.Context) error {
	listening := false
	defer func() {
		if listening {
			w.metrics.ListenerUp.Set(0)
		}
	}()

	return w.listener.Listen(ctx, func() {
		if !listening {
			listening = true
			w.relisten.Reset()
			w.metrics.ListenerUp.Set(1)
			w.log.Info("outbox listener up; new messages wake the worker")
		}
		w.signal()
	})
}

// signal leaves one wake-up pending; a second one would add nothing.
func (w *Wakeup) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}
