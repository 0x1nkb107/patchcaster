package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePart stands in for a listener or background task: it runs until its
// context ends and remembers that context.
type fakePart struct {
	mu      sync.Mutex
	ctx     context.Context
	started chan struct{}
	fail    error // returned at once instead of blocking, when set
}

func newFakePart() *fakePart { return &fakePart{started: make(chan struct{})} }

func (p *fakePart) run(ctx context.Context) error {
	p.mu.Lock()
	p.ctx = ctx
	p.mu.Unlock()
	close(p.started)
	if p.fail != nil {
		return p.fail
	}
	<-ctx.Done()
	return nil
}

// ended reports whether the part's context has been cancelled.
func (p *fakePart) ended() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ctx.Err() != nil
}

// fakeWorker stands in for the worker in the middle of a delivery: Run blocks
// until the test finishes the delivery or the context ends.
type fakeWorker struct {
	mu       sync.Mutex
	ctx      context.Context
	started  chan struct{}
	stopped  chan struct{} // closed by StopAfterCurrentJob
	finish   chan struct{} // closed by the test to complete the delivery
	onStop   func()        // runs inside StopAfterCurrentJob
	onFinish func()        // runs just before a finished delivery returns
}

func newFakeWorker() *fakeWorker {
	return &fakeWorker{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
		finish:  make(chan struct{}),
	}
}

func (w *fakeWorker) Run(ctx context.Context) error {
	w.mu.Lock()
	w.ctx = ctx
	w.mu.Unlock()
	close(w.started)
	select {
	case <-w.finish:
		if w.onFinish != nil {
			w.onFinish()
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *fakeWorker) StopAfterCurrentJob() {
	if w.onStop != nil {
		w.onStop()
	}
	close(w.stopped)
}

// cancelled reports whether the worker's context has been cancelled.
func (w *fakeWorker) cancelled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ctx.Err() != nil
}

// lifecycleHarness wires a lifecycle over fakes and records readiness.
type lifecycleHarness struct {
	internal   *fakePart
	public     *fakePart
	background *fakePart
	worker     *fakeWorker
	readiness  []bool // written by run's goroutine only
	lc         *lifecycle
}

func newLifecycleHarness(drainBudget time.Duration) *lifecycleHarness {
	h := &lifecycleHarness{
		internal:   newFakePart(),
		public:     newFakePart(),
		background: newFakePart(),
		worker:     newFakeWorker(),
	}
	h.lc = &lifecycle{
		internal:    h.internal.run,
		public:      h.public.run,
		worker:      h.worker,
		background:  []func(context.Context) error{h.background.run},
		markReady:   func(ready bool) { h.readiness = append(h.readiness, ready) },
		drainBudget: drainBudget,
		log:         slog.New(slog.DiscardHandler),
	}
	return h
}

// start runs the lifecycle and waits until every part is up.
func (h *lifecycleHarness) start(t *testing.T, ctx context.Context) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- h.lc.run(ctx) }()
	for _, started := range []chan struct{}{
		h.internal.started, h.public.started, h.background.started, h.worker.started,
	} {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("a part never started")
		}
	}
	return done
}

// awaitReturn returns run's result, failing the test on a hang.
func awaitReturn(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return")
		return nil
	}
}

func TestLifecycle_FailedPartTearsDownTheRest(t *testing.T) {
	// arrange: the internal listener fails as soon as it starts.
	h := newLifecycleHarness(time.Minute)
	h.internal.fail = errors.New("serve internal listener: boom")

	// act
	done := h.start(t, context.Background())
	err := awaitReturn(t, done)

	// assert: the failure is reported and every other part was stopped.
	require.EqualError(t, err, "serve internal listener: boom")
	assert.True(t, h.public.ended())
	assert.True(t, h.background.ended())
	assert.True(t, h.worker.cancelled())
	assert.Equal(t, []bool{true, false}, h.readiness)
}
