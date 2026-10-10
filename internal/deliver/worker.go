package deliver

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/0x5a17ed/patchcaster/internal/event"
	"github.com/0x5a17ed/patchcaster/internal/mattermost"
	"github.com/0x5a17ed/patchcaster/internal/metrics"
	"github.com/0x5a17ed/patchcaster/internal/store"
)

// Owner-approved retry constants (ADR-008): 1 s start, doubling, capped at
// 10 min, 8 attempts total, then park. Jitter is disabled so the deferral
// ladder (1s, 2s, 4s ... 600s) is exact and testable.
const (
	retryInitial    = time.Second
	retryMultiplier = 2.0
	retryMax        = 10 * time.Minute
	maxAttempts     = 8
)

// storeFaultDelay requeues a job after a local store failure without burning
// the delivery attempt budget: a store outage must never drop messages
// (SYSREQ-007 acceptance criteria).
const storeFaultDelay = 5 * time.Second

// Poster is the Mattermost operation the worker performs.
type Poster interface {
	Post(ctx context.Context, channelID, message, rootID string) (string, error)
}

// Store is the queue and mapping surface the worker claims and settles on.
type Store interface {
	ClaimDue(ctx context.Context, now time.Time, lease time.Duration) (*store.Job, error)
	Settle(ctx context.Context, jobID int64, now time.Time, s store.Settlement) error
	Thread(ctx context.Context, mrRef string) (*store.Thread, error)
	OpenThread(ctx context.Context, mrRef, channelID, rootPostID string, now time.Time) error
	CloseThread(ctx context.Context, mrRef string, now time.Time) error
	ReapLeases(ctx context.Context, now time.Time) error
}

// Config tunes the claim loop.
type Config struct {
	Poll  time.Duration // idle wait between claim attempts
	Lease time.Duration // claim lease before the reaper may requeue
}

// Worker delivers queued messages to Mattermost: one thread root per merge
// request, every later action as a reply (SYSREQ-004/005).
//
// A single worker goroutine is run per replica (v1 deployment is one replica
// by choice, ADR-010). Claiming in arrival order then serializes all activity
// per merge request, which is what makes "exactly one root post" robust.
type Worker struct {
	store    Store
	poster   Poster
	renderer *Renderer
	metrics  *metrics.Metrics
	log      *slog.Logger
	cfg      Config
	now      func() time.Time

	// stopping requests a graceful end: finish the job in hand, claim
	// nothing more. Context cancellation stays the hard stop.
	stopping atomic.Bool
}

// StopAfterCurrentJob ends the claim loop once the in-flight delivery
// completes (drain, SYSREQ-023).
func (w *Worker) StopAfterCurrentJob() { w.stopping.Store(true) }

// NewWorker wires a worker; a nil clock defaults to time.Now.
func NewWorker(st Store, poster Poster, r *Renderer, m *metrics.Metrics, log *slog.Logger, cfg Config, now func() time.Time) *Worker {
	if now == nil {
		now = time.Now
	}
	if cfg.Poll <= 0 {
		cfg.Poll = time.Second
	}
	if cfg.Lease <= 0 {
		cfg.Lease = time.Minute
	}
	return &Worker{store: st, poster: poster, renderer: r, metrics: m, log: log, cfg: cfg, now: now}
}

// Run claims and delivers until ctx ends. Mid-delivery work is abandoned
// promptly; its lease is reaped for the next worker (or the next start).
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.Poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if w.stopping.Load() {
			return nil
		}

		if err := w.store.ReapLeases(ctx, w.now()); err != nil {
			w.log.Error("lease reap failed", "err", err)
			continue
		}
		job, err := w.store.ClaimDue(ctx, w.now(), w.cfg.Lease)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("claim failed", "err", err)
			continue
		}
		if job == nil {
			continue
		}
		w.process(ctx, job)
		if w.stopping.Load() {
			return nil
		}
	}
}

// process settles one claimed job. It never returns an error: every outcome
// (sent, skipped, retried, parked) is recorded on the row so no delivery is
// lost or spun.
func (w *Worker) process(ctx context.Context, job *store.Job) {
	ev, err := job.Event()
	if err != nil {
		// The row was written by our own encoder; undecodable payload means
		// stored data or code drifted. Nothing retryable here: park it.
		w.log.Error("job payload undecodable, parking", "job", job.ID, "err", err)
		w.settle(ctx, job, store.Parked("payload_undecodable", job.Attempts+1, err.Error()))
		w.metrics.Parked.WithLabelValues("payload_undecodable").Inc()
		return
	}

	log := w.log.With("job", job.ID, "mr", ev.MRRef, "action", ev.Action, "project", ev.ProjectPath)
	if ev.Action == "open" {
		w.deliverRoot(ctx, job, ev, log)
		return
	}
	w.deliverReply(ctx, job, ev, log)
}

// deliverRoot posts the thread-opening message. An existing mapping means the
// root is already announced (or the thread is closed), so a redelivery must
// never invent a second root (SYSREQ-002/007).
func (w *Worker) deliverRoot(ctx context.Context, job *store.Job, ev *event.Event, log *slog.Logger) {
	thread, err := w.store.Thread(ctx, ev.MRRef)
	if err != nil {
		log.Error("thread mapping lookup failed", "err", err)
		w.settle(ctx, job, store.Retried(job.Attempts, w.now().Add(storeFaultDelay), err.Error()))
		return
	}
	if thread != nil {
		cause := metrics.CauseDuplicateOpen
		if !thread.Open {
			cause = metrics.CauseThreadClosed
		}
		log.Info("root already announced, skipping", "cause", cause)
		w.settle(ctx, job, store.Skipped(cause))
		w.metrics.Skipped.WithLabelValues(cause).Inc()
		return
	}

	message, err := w.renderer.Root(ev)
	if err != nil {
		log.Error("render failed, parking", "err", err)
		w.settle(ctx, job, store.Parked("render_failed", job.Attempts+1, err.Error()))
		w.metrics.Parked.WithLabelValues("render_failed").Inc()
		return
	}

	postID, err := w.post(ctx, job.DestChannel, message, "")
	if err != nil {
		w.deferOrFail(ctx, job, err, log)
		return
	}

	// The post is accepted; recording the mapping is the known at-least-once
	// window. On failure we leave the row claimed and let the lease reaper
	// requeue it — a rare duplicate root beats a lost thread mapping.
	if err := w.store.OpenThread(ctx, ev.MRRef, job.DestChannel, postID, w.now()); err != nil {
		log.Error("root posted but mapping persist failed; leaving for lease reap", "err", err)
		return
	}
	log.Info("root post announced", "post", postID, "channel", job.DestChannel)
	w.settle(ctx, job, store.Sent())
}

// deliverReply posts every post-open action into the stored thread. Without a
// mapping the MR predates this deployment: skip, log, count (SYSREQ-007).
// The reply always targets the thread's own channel, which is authoritative
// even if routing changed after the root was posted.
func (w *Worker) deliverReply(ctx context.Context, job *store.Job, ev *event.Event, log *slog.Logger) {
	thread, err := w.store.Thread(ctx, ev.MRRef)
	if err != nil {
		log.Error("thread mapping lookup failed", "err", err)
		w.settle(ctx, job, store.Retried(job.Attempts, w.now().Add(storeFaultDelay), err.Error()))
		return
	}
	if thread == nil {
		log.Info("no thread mapping, skipping", "cause", metrics.CauseNoMapping)
		w.settle(ctx, job, store.Skipped(metrics.CauseNoMapping))
		w.metrics.Skipped.WithLabelValues(metrics.CauseNoMapping).Inc()
		return
	}
	if !thread.Open {
		log.Info("thread closed, skipping", "cause", metrics.CauseThreadClosed)
		w.settle(ctx, job, store.Skipped(metrics.CauseThreadClosed))
		w.metrics.Skipped.WithLabelValues(metrics.CauseThreadClosed).Inc()
		return
	}

	message, err := w.renderer.Reply(ev)
	if err != nil {
		log.Error("render failed, parking", "err", err)
		w.settle(ctx, job, store.Parked("render_failed", job.Attempts+1, err.Error()))
		w.metrics.Parked.WithLabelValues("render_failed").Inc()
		return
	}

	if _, err := w.post(ctx, thread.ChannelID, message, thread.RootPostID); err != nil {
		var gone *mattermost.ThreadGoneError
		if errors.As(err, &gone) {
			// The root was deleted intentionally and stays deleted (SYSREQ-026):
			// park this message and close the mapping forever.
			log.Warn("thread root deleted, parking and closing mapping")
			if err := w.store.CloseThread(ctx, ev.MRRef, w.now()); err != nil {
				log.Error("closing mapping failed; leaving for lease reap", "err", err)
				return
			}
			w.settle(ctx, job, store.Parked(metrics.CauseThreadGone, job.Attempts+1, err.Error()))
			w.metrics.Parked.WithLabelValues(metrics.CauseThreadGone).Inc()
			return
		}
		w.deferOrFail(ctx, job, err, log)
		return
	}
	log.Info("reply posted", "root", thread.RootPostID, "channel", thread.ChannelID)
	w.settle(ctx, job, store.Sent())
}

// post performs one Mattermost attempt with metrics.
func (w *Worker) post(ctx context.Context, channelID, message, rootID string) (string, error) {
	start := time.Now()
	defer func() { w.metrics.MMDuration.Observe(time.Since(start).Seconds()) }()

	postID, err := w.poster.Post(ctx, channelID, message, rootID)
	if err != nil {
		w.recordAttemptOutcome(err)
		return "", err
	}
	w.metrics.MMAttempts.WithLabelValues(metrics.OutcomeSent).Inc()
	w.metrics.Posted.Inc()
	return postID, nil
}

// deferOrFail schedules the next attempt or parks on budget exhaustion
// (SYSREQ-016/018). A rate limit's Reset instant outranks the backoff ladder
// when it is later.
func (w *Worker) deferOrFail(ctx context.Context, job *store.Job, causeErr error, log *slog.Logger) {
	attempts := job.Attempts + 1
	if attempts >= maxAttempts {
		log.Warn("attempt budget exhausted, parking", "attempts", attempts, "err", causeErr)
		w.settle(ctx, job, store.Parked(metrics.CauseBudgetExched, attempts, causeErr.Error()))
		w.metrics.Parked.WithLabelValues(metrics.CauseBudgetExched).Inc()
		return
	}

	next := w.now().Add(backoffInterval(attempts))
	var rl *mattermost.RateLimitedError
	if errors.As(causeErr, &rl) && rl.Reset.After(next) {
		next = rl.Reset
	}
	log.Warn("delivery deferred", "attempts", attempts, "next_attempt", next, "err", causeErr)
	w.settle(ctx, job, store.Retried(attempts, next, causeErr.Error()))
}

// recordAttemptOutcome buckets the error kind for the attempts counter.
func (w *Worker) recordAttemptOutcome(err error) {
	switch {
	case errors.As(err, new(*mattermost.RateLimitedError)):
		w.metrics.MMAttempts.WithLabelValues(metrics.OutcomeRateLimited).Inc()
	case errors.As(err, new(*mattermost.ThreadGoneError)):
		w.metrics.MMAttempts.WithLabelValues(metrics.OutcomeThreadGone).Inc()
	case errors.As(err, new(*mattermost.TransportError)):
		w.metrics.MMAttempts.WithLabelValues(metrics.OutcomeTransport).Inc()
	default:
		w.metrics.MMAttempts.WithLabelValues(metrics.OutcomeRetryable).Inc()
	}
}

func (w *Worker) settle(ctx context.Context, job *store.Job, s store.Settlement) {
	if err := w.store.Settle(ctx, job.ID, w.now(), s); err != nil {
		// Nothing safer than waiting: the lease reaper will requeue.
		w.log.Error("settle failed; leaving for lease reap", "job", job.ID, "err", err)
	}
}

// backoffInterval returns the deferral after the n-th failed attempt using
// the ADR-008 ladder. A fresh, jitter-free generator is stepped n times, so
// the result depends only on n.
func backoffInterval(n int) time.Duration {
	b := &backoff.ExponentialBackOff{
		InitialInterval:     retryInitial,
		RandomizationFactor: 0,
		Multiplier:          retryMultiplier,
		MaxInterval:         retryMax,
	}
	b.Reset()
	var d time.Duration
	for i := 0; i < n; i++ {
		d = b.NextBackOff()
	}
	return d
}
