package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/graceful"
)

// collect receives n results from errc.
func collect(errc chan error, n int) []error {
	out := make([]error, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, <-errc)
	}
	return out
}

// record keeps err unless it only reports an orderly stop.
func record(err error, errs *[]error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, graceful.ErrStopped) {
		return
	}
	*errs = append(*errs, err)
}

func recordAll(errs []error, into *[]error) {
	for _, err := range errs {
		record(err, into)
	}
}

// drainingWorker is the delivery worker as the shutdown sequence drives it.
type drainingWorker interface {
	Run(ctx context.Context) error
	StopAfterCurrentJob()
}

// lifecycle runs the long-running parts of serve and owns their shutdown
// order (SYSREQ-023, SYSARCH-004).
type lifecycle struct {
	internal    func(context.Context) error   // probes and metrics listener
	public      func(context.Context) error   // webhook listener
	worker      drainingWorker                // delivery worker
	background  []func(context.Context) error // tasks that end with the worker
	markReady   func(bool)
	drainBudget time.Duration
	log         *slog.Logger
}

// run starts every part, marks the instance ready, and blocks until ctx ends
// or a part fails. A failed part tears everything down at once. When ctx
// ends, readiness flips false, the public listener stops accepting, and the
// worker gets the drain budget to finish what is in flight before the
// internal listener stops last.
func (l *lifecycle) run(ctx context.Context) error {
	// ctx only signals the shutdown; it must not end the parts itself.
	// Parts derived from it would all stop the moment SIGTERM arrives,
	// cancelling the delivery in hand and the probes with it. Only the
	// sequence below ends them, each at its turn.
	detached := context.WithoutCancel(ctx)
	pubCtx, cancelPub := context.WithCancel(detached)
	wrkCtx, cancelWrk := context.WithCancel(detached)
	intCtx, cancelInt := context.WithCancel(detached)
	// Belt for early-return paths; the shutdown choreography below cancels
	// these deliberately, and repeated cancel is a no-op.
	defer cancelPub()
	defer cancelWrk()
	defer cancelInt()

	// One buffered slot per part so late returns never block on a channel
	// nobody drains during shutdown.
	parts := 3 + len(l.background)
	errc := make(chan error, parts)
	workerDone := make(chan struct{})
	go func() { errc <- l.internal(intCtx) }()
	go func() { errc <- l.public(pubCtx) }()
	go func() {
		err := l.worker.Run(wrkCtx)
		close(workerDone)
		errc <- err
	}()
	for _, task := range l.background {
		go func() { errc <- task(wrkCtx) }()
	}

	l.markReady(true)

	var errs []error
	select {
	case err := <-errc:
		// A part failed, at startup or later: tear everything down at once.
		record(err, &errs)
		cancelPub()
		cancelWrk()
		cancelInt()
		recordAll(collect(errc, parts-1), &errs)

	case <-ctx.Done():
		l.markReady(false)
		l.log.Info("shutdown signaled", "drain_budget", l.drainBudget)
		cancelPub() // webhooks stop being accepted immediately
		l.worker.StopAfterCurrentJob()

		select {
		case <-workerDone:
			l.log.Info("worker finished within drain budget")
		case <-time.After(l.drainBudget):
			l.log.Warn("drain budget exhausted; releasing worker")
			cancelWrk()
			<-workerDone
		}
		cancelWrk() // background tasks end with the worker
		cancelInt()
		recordAll(collect(errc, parts), &errs)
	}

	l.markReady(false)
	return errors.Join(errs...)
}
