package deliver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/mattermost"
	"github.com/patchcaster-app/patchcaster/internal/metrics"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

var workerNow = time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)

// --- fakes -------------------------------------------------------------------

type postCall struct {
	channel string
	message string
	rootID  string
}

type postResult struct {
	postID string
	err    error
}

type updateCall struct {
	postID  string
	message string
}

type fakePoster struct {
	calls      []postCall
	results    []postResult // consumed FIFO; default success "post-ok"
	updates    []updateCall
	updateErrs []error // consumed FIFO; default success
}

func (f *fakePoster) Post(_ context.Context, channel, message, rootID string) (string, error) {
	f.calls = append(f.calls, postCall{channel: channel, message: message, rootID: rootID})
	if len(f.results) > 0 {
		r := f.results[0]
		f.results = f.results[1:]
		return r.postID, r.err
	}
	return "post-ok", nil
}

func (f *fakePoster) Update(_ context.Context, postID, message string) error {
	f.updates = append(f.updates, updateCall{postID: postID, message: message})
	if len(f.updateErrs) > 0 {
		err := f.updateErrs[0]
		f.updateErrs = f.updateErrs[1:]
		return err
	}
	return nil
}

type settleRec struct {
	jobID int64
	s     store.Settlement
}

type fakeStore struct {
	jobs        []*store.Job
	thread      *store.Thread
	errThread   error
	state       *store.State
	errState    error
	settles     []settleRec
	opened      []string // "mr|channel|root"
	closed      []string
	stateClosed []string
	noticeFlags []int64
	rootFlags   []int64
	settledCh   chan struct{} // pinged on every settle when non-nil
}

func (f *fakeStore) ClaimDue(_ context.Context, _ time.Time, _ time.Duration) (*store.Job, error) {
	if len(f.jobs) == 0 {
		return nil, nil
	}
	job := f.jobs[0]
	f.jobs = f.jobs[1:]
	return job, nil
}

func (f *fakeStore) Settle(_ context.Context, jobID int64, _ time.Time, s store.Settlement) error {
	f.settles = append(f.settles, settleRec{jobID: jobID, s: s})
	if f.settledCh != nil {
		select {
		case f.settledCh <- struct{}{}:
		default:
		}
	}
	return nil
}

func (f *fakeStore) Thread(context.Context, string) (*store.Thread, error) {
	return f.thread, f.errThread
}

// OpenThread mirrors Postgres behavior: after a root is mapped, later claims
// in the same test run must observe the mapping.
func (f *fakeStore) OpenThread(_ context.Context, mrRef, channelID, rootPostID string, _ time.Time) error {
	f.opened = append(f.opened, mrRef+"|"+channelID+"|"+rootPostID)
	if f.thread == nil {
		f.thread = &store.Thread{ChannelID: channelID, RootPostID: rootPostID, Open: true}
	}
	return nil
}

func (f *fakeStore) CloseThread(_ context.Context, mrRef string, _ time.Time) error {
	f.closed = append(f.closed, mrRef)
	if f.thread != nil {
		f.thread.Open = false
	}
	return nil
}

func (f *fakeStore) CloseState(_ context.Context, mrRef string, _ time.Time) error {
	f.stateClosed = append(f.stateClosed, mrRef)
	return nil
}

func (f *fakeStore) State(context.Context, string) (*store.State, error) {
	return f.state, f.errState
}

func (f *fakeStore) MarkNoticePosted(_ context.Context, jobID int64, _ time.Time) error {
	f.noticeFlags = append(f.noticeFlags, jobID)
	return nil
}

func (f *fakeStore) MarkRootRefreshed(_ context.Context, jobID int64, _ time.Time) error {
	f.rootFlags = append(f.rootFlags, jobID)
	return nil
}

func (f *fakeStore) ReapLeases(context.Context, time.Time) error { return nil }

// --- harness -------------------------------------------------------------------

func openJob(attempts int) *store.Job {
	return openJobFor(attempts, "open")
}

func openJobFor(attempts int, action string) *store.Job {
	ev := &event.Event{Kind: event.KindMergeRequest,
		Action: action, MRRef: "987", IID: 12, ProjectPath: "grp/app",
		Title: "T", Author: "ada", Actor: "ada", URL: "https://x.example/1",
	}
	return queuedJob(1, attempts, "chan-dest", ev)
}

func queuedJob(id int64, attempts int, dest string, ev *event.Event) *store.Job {
	job := &store.Job{ID: id, MRRef: ev.MRRef, Action: ev.Action, DestChannel: dest, Attempts: attempts}
	b, err := json.Marshal(ev)
	if err != nil {
		panic(err)
	}
	job.Payload = b
	return job
}

type harness struct {
	w  *Worker
	fs *fakeStore
	fp *fakePoster
	m  *metrics.Metrics
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	r, err := NewRenderer()
	require.NoError(t, err)
	h := &harness{fs: &fakeStore{}, fp: &fakePoster{}, m: metrics.New()}
	h.w = NewWorker(h.fs, h.fp, r, h.m, slog.New(slog.DiscardHandler),
		Config{Poll: 10 * time.Millisecond, Lease: time.Minute},
		func() time.Time { return workerNow })
	return h
}

// --- lifecycle decisions ---------------------------------------------------------

func TestWorker_RootPostedOnceAndMapped(t *testing.T) {
	// arrange
	h := newHarness(t)

	// act
	h.w.process(context.Background(), openJob(0))

	// assert
	require.Len(t, h.fp.calls, 1)
	assert.Equal(t, "chan-dest", h.fp.calls[0].channel, "root goes to the frozen destination")
	assert.Empty(t, h.fp.calls[0].rootID, "root post is not itself a reply")
	assert.Equal(t, []string{"987|chan-dest|post-ok"}, h.fs.opened)
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
	assert.Equal(t, 1.0, testutil.ToFloat64(h.m.Posted))
}

func TestWorker_DuplicateRootSkips(t *testing.T) {
	// arrange: mapping exists although the action is "open" again.
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "r", Open: true}

	// act
	h.w.process(context.Background(), openJob(0))

	// assert: never invent a second root (SYSREQ-002).
	assert.Empty(t, h.fp.calls)
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementSkipped, h.fs.settles[0].s.Kind)
	assert.Equal(t, metrics.CauseDuplicateOpen, h.fs.settles[0].s.Reason)
	assert.Equal(t, 1.0, testutil.ToFloat64(h.m.Skipped.WithLabelValues(metrics.CauseDuplicateOpen)))
}

func TestWorker_ReplyUsesThreadChannelNotDestination(t *testing.T) {
	// arrange: routing moved after the root existed; the thread is home.
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "chan-thread", RootPostID: "root-9", Open: true}
	job := queuedJob(2, 0, "chan-dest-new", &event.Event{Kind: event.KindMergeRequest,
		Action: "approval", MRRef: "987", IID: 12, ProjectPath: "grp/app",
		Title: "T", Author: "ada", Actor: "grace", URL: "https://x.example/1",
	})

	// act
	h.w.process(context.Background(), job)

	// assert
	require.Len(t, h.fp.calls, 1)
	assert.Equal(t, "chan-thread", h.fp.calls[0].channel)
	assert.Equal(t, "root-9", h.fp.calls[0].rootID, "notices reply into the stored thread (SYSREQ-005)")
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
}

func TestWorker_ReplyWithoutMappingSkips(t *testing.T) {
	// arrange: MR opened before this deployment knew the project (SYSREQ-007).
	h := newHarness(t)
	h.fs.thread = nil

	// act
	h.w.process(context.Background(), queuedJob(2, 0, "chan-dest", &event.Event{Kind: event.KindMergeRequest,
		Action: "merge", MRRef: "987", IID: 12, ProjectPath: "grp/app",
	}))

	// assert
	assert.Empty(t, h.fp.calls, "no root may be invented for a late first event")
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementSkipped, h.fs.settles[0].s.Kind)
	assert.Equal(t, metrics.CauseNoMapping, h.fs.settles[0].s.Reason)
}

func TestWorker_ClosedThreadSkips(t *testing.T) {
	// arrange
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "r", Open: false}

	// act
	h.w.process(context.Background(), queuedJob(2, 0, "chan-dest", &event.Event{Kind: event.KindMergeRequest, Action: "merge", MRRef: "987"}))

	// assert
	assert.Empty(t, h.fp.calls)
	assert.Equal(t, metrics.CauseThreadClosed, h.fs.settles[0].s.Reason)
}

func TestWorker_DeletedRootParksAndClosesForever(t *testing.T) {
	// arrange: Mattermost reports the root post gone (SYSREQ-026).
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "gone", Open: true}
	h.fp.results = []postResult{{err: &mattermost.ThreadGoneError{}}}

	// act
	h.w.process(context.Background(), noticeJob(2, 0, "chan-dest"))

	// assert
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementParked, h.fs.settles[0].s.Kind)
	assert.Equal(t, metrics.CauseThreadGone, h.fs.settles[0].s.Reason)
	assert.Equal(t, []string{"987"}, h.fs.closed, "mapping must stay closed afterwards")
	assert.Equal(t, 1.0, testutil.ToFloat64(h.m.Parked.WithLabelValues(metrics.CauseThreadGone)))
}

// --- retry policy ----------------------------------------------------------------

func TestWorker_RetryableErrorDefersByBackoffLadder(t *testing.T) {
	ladder := []struct {
		claimedAttempts int
		wantDelay       time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
	}
	for i, tc := range ladder {
		// arrange
		h := newHarness(t)
		h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "r", Open: true}
		h.fp.results = []postResult{{err: &mattermost.APIError{Status: 500, Body: "boom"}}}

		// act
		h.w.process(context.Background(), queuedJob(int64(i), tc.claimedAttempts, "chan-dest",
			&event.Event{Kind: event.KindMergeRequest, Action: "approval", MRRef: "987", IID: 12}))

		// assert
		require.Len(t, h.fs.settles, 1)
		s := h.fs.settles[0].s
		require.Equal(t, store.SettlementRetried, s.Kind)
		assert.Equal(t, tc.claimedAttempts+1, s.Attempts)
		assert.True(t, s.Next.Equal(workerNow.Add(tc.wantDelay)),
			"attempt %d: want %s, got %s", tc.claimedAttempts+1, tc.wantDelay, s.Next.Sub(workerNow))
	}
}

func TestWorker_BudgetExhaustionParks(t *testing.T) {
	// arrange: the 8th consecutive failure (7 recorded + this one).
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "r", Open: true}
	h.fp.results = []postResult{{err: &mattermost.APIError{Status: 502, Body: "bad gateway"}}}

	// act
	h.w.process(context.Background(), queuedJob(9, 7, "chan-dest", &event.Event{Kind: event.KindMergeRequest, Action: "approval", MRRef: "987", IID: 12}))

	// assert
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementParked, h.fs.settles[0].s.Kind)
	assert.Equal(t, metrics.CauseBudgetExched, h.fs.settles[0].s.Reason)
	assert.Equal(t, 8, h.fs.settles[0].s.Attempts)
}

func TestWorker_RateLimitResetOutranksBackoff(t *testing.T) {
	// arrange: reset far in the future of the 1 s ladder step.
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "r", Open: true}
	reset := workerNow.Add(30 * time.Minute)
	h.fp.results = []postResult{{err: &mattermost.RateLimitedError{Reset: reset}}}

	// act
	h.w.process(context.Background(), openJobFor(0, "approval"))

	// assert
	s := h.fs.settles[0].s
	require.Equal(t, store.SettlementRetried, s.Kind)
	assert.True(t, s.Next.Equal(reset), "schedule must honor RateLimit-Reset when later (SYSREQ-018)")
	assert.Equal(t, 1.0, testutil.ToFloat64(h.m.MMAttempts.WithLabelValues(metrics.OutcomeRateLimited)))
}

func TestWorker_BackoffOutranksNearReset(t *testing.T) {
	// arrange: ladder says 32 s (attempt 6), server reset is only 5 s away.
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "r", Open: true}
	h.fp.results = []postResult{{err: &mattermost.RateLimitedError{Reset: workerNow.Add(5 * time.Second)}}}

	// act
	h.w.process(context.Background(), openJobFor(5, "approval"))

	// assert
	assert.True(t, h.fs.settles[0].s.Next.Equal(workerNow.Add(32*time.Second)),
		"later of ladder and reset wins")
}

func TestWorker_StoreFaultDoesNotBurnBudget(t *testing.T) {
	// arrange: local store broken while deciding (SYSREQ-007: never drop).
	h := newHarness(t)
	h.fs.errThread = errors.New("store down")

	// act
	h.w.process(context.Background(), queuedJob(3, 4, "chan-dest", &event.Event{Kind: event.KindMergeRequest, Action: "update", MRRef: "987"}))

	// assert: requeued at the fixed store-fault delay, attempts unchanged.
	s := h.fs.settles[0].s
	require.Equal(t, store.SettlementRetried, s.Kind)
	assert.Equal(t, 4, s.Attempts)
	assert.True(t, s.Next.Equal(workerNow.Add(storeFaultDelay)))
}

func TestWorker_UndecodablePayloadParks(t *testing.T) {
	// arrange
	h := newHarness(t)
	job := &store.Job{ID: 1, MRRef: "987", Action: "open", DestChannel: "c", Attempts: 2, Payload: []byte("{broken")}

	// act
	h.w.process(context.Background(), job)

	// assert
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementParked, h.fs.settles[0].s.Kind)
	assert.Equal(t, "payload_undecodable", h.fs.settles[0].s.Reason)
	assert.Equal(t, 3, h.fs.settles[0].s.Attempts)
}

// --- claim loop -------------------------------------------------------------------

func TestWorker_RunDeliversInClaimOrder(t *testing.T) {
	// arrange: root event queued before its update; the loop must post them
	// in that order, reply attached to the root that was just mapped.
	h := newHarness(t)
	h.fs.settledCh = make(chan struct{}, 4)
	ev := &event.Event{Kind: event.KindMergeRequest, Action: "open", MRRef: "987", IID: 12, ProjectPath: "grp/app",
		Title: "T", Author: "ada", Actor: "ada", URL: "https://x/1"}
	h.fs.jobs = []*store.Job{queuedJob(1, 0, "chan-dest", ev)}
	update := *ev
	update.Action = "merge"
	h.fs.jobs = append(h.fs.jobs, queuedJob(2, 0, "chan-dest", &update))

	// The fake store mirrors OpenThread into its mapping, exactly as Postgres
	// would make it visible to the next claim.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.w.Run(ctx) }()

	// act: wait for both settles.
	for i := 0; i < 2; i++ {
		select {
		case <-h.fs.settledCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("settle %d never happened", i+1)
		}
	}
	cancel()

	// assert
	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, h.fp.calls, 2)
	assert.Empty(t, h.fp.calls[0].rootID)
	assert.Equal(t, "post-ok", h.fp.calls[1].rootID, "update rode the thread opened by the open event")
	assert.Equal(t, 2.0, testutil.ToFloat64(h.m.Posted))
}

// --- schedule unit ----------------------------------------------------------------

func TestWorker_StopAfterCurrentJobExitsCleanly(t *testing.T) {
	// arrange
	h := newHarness(t)
	h.fs.settledCh = make(chan struct{}, 1)
	h.fs.jobs = []*store.Job{openJob(0)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.w.Run(ctx) }()

	// act: request stop while the job settles.
	<-h.fs.settledCh
	h.w.StopAfterCurrentJob()

	// assert: graceful nil return, not a context error; the job was delivered.
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit after stop request")
	}
	require.Len(t, h.fp.calls, 1)
}

func TestBackoffIntervalLadder(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{10, 512 * time.Second},
		{11, 10 * time.Minute}, // cap reached (ADR-008)
		{12, 10 * time.Minute},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, backoffInterval(tc.attempts), "attempt %d", tc.attempts)
	}
}

// --- two-phase delivery (SYSREQ-028/029, ADR-014) ----------------------------------

// planJob queues a comment-kind job carrying an explicit delivery plan; the
// worker executes plans, it never re-reads GitLab dialect.
func planJob(id int64, attempts int, p *event.Plan) *store.Job {
	ev := &event.Event{
		Kind: event.KindNote, Action: "create", MRRef: "987", IID: 12, ProjectPath: "grp/app",
		Actor: "bob", Title: "T", Author: "ada", URL: "https://x.example/1", Plan: p,
	}
	return queuedJob(id, attempts, "chan-dest", ev)
}

func noticeJob(id int64, attempts int, dest string) *store.Job {
	job := planJob(id, attempts, &event.Plan{Notice: plan.Comment})
	job.DestChannel = dest
	return job
}

func openBoardThread(h *harness) {
	h.fs.thread = &store.Thread{ChannelID: "chan-thread", RootPostID: "root-9", Open: true}
	h.fs.state = &store.State{
		MrRef: "987", Title: "T", Author: "ada", URL: "https://x.example/1", IID: 12,
		ProjectPath: "grp/app", SourceBranch: "feat", TargetBranch: "main",
		PipelineStatus: "success", Comments: 3, ReviewersDone: 1, ReviewersRequired: 2,
	}
}

const wantRefreshedRoot = "> @here :bell: \\\n" +
	"> **!12 T** \\\n" +
	"> by @ada • grp/app • `feat` ➜ `main` \\\n" +
	"> https://x.example/1 \\\n" +
	"> 🟢 Pipeline passed • 👀 1/2 reviewers • 💬 3 comments"

func TestWorker_NoticeThenRefreshInOneDelivery(t *testing.T) {
	// arrange: a notifying delivery with both phases pending.
	h := newHarness(t)
	openBoardThread(h)

	// act
	h.w.process(context.Background(), planJob(3, 0, &event.Plan{Notice: plan.Comment, Refresh: true}))

	// assert: one reply, one root PUT with the current board, both phases flagged.
	require.Len(t, h.fp.calls, 1)
	assert.Equal(t, "chan-thread", h.fp.calls[0].channel)
	assert.Equal(t, "root-9", h.fp.calls[0].rootID)
	assert.Equal(t, "💬 bob commented", h.fp.calls[0].message)
	require.Len(t, h.fp.updates, 1)
	assert.Equal(t, "root-9", h.fp.updates[0].postID)
	assert.Equal(t, wantRefreshedRoot, h.fp.updates[0].message)
	assert.Equal(t, []int64{3}, h.fs.noticeFlags)
	assert.Equal(t, []int64{3}, h.fs.rootFlags)
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
	assert.Equal(t, 1.0, testutil.ToFloat64(h.m.Notices.WithLabelValues(plan.Comment)))
}

func TestWorker_CompletedPhasesAreNotRepeated(t *testing.T) {
	// arrange: a retry of a delivery whose notice already reached Mattermost.
	h := newHarness(t)
	openBoardThread(h)
	job := planJob(4, 1, &event.Plan{Notice: plan.Comment, Refresh: true})
	job.NoticePosted = true

	// act
	h.w.process(context.Background(), job)

	// assert: no second notice (SYSREQ-028), the refresh proceeds.
	assert.Empty(t, h.fp.calls, "a posted notice is never reposted on retry")
	require.Len(t, h.fp.updates, 1)
	assert.Equal(t, []int64{4}, h.fs.rootFlags)
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
}

func TestWorker_BothPhasesDoneOnlySettles(t *testing.T) {
	// arrange: crash after both operations, before settle — the requeue must
	// neither post nor PUT again.
	h := newHarness(t)
	openBoardThread(h)
	job := planJob(5, 2, &event.Plan{Notice: plan.Comment, Refresh: true})
	job.NoticePosted, job.RootRefreshed = true, true

	// act
	h.w.process(context.Background(), job)

	// assert
	assert.Empty(t, h.fp.calls)
	assert.Empty(t, h.fp.updates)
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
}

func TestWorker_MergeOrdersNoticeRefreshThenClose(t *testing.T) {
	// arrange: SYSREQ-028 — notice precedes final refresh precedes close.
	// Accept sets Refresh (Notice != ""), so the queued plan carries all three.
	h := newHarness(t)
	openBoardThread(h)
	merge := &event.Event{Kind: event.KindMergeRequest, Action: "merge", MRRef: "987", IID: 12, Actor: "ada"}
	merge.Plan = &event.Plan{Notice: plan.Merged, Refresh: true, CloseAfter: true}
	h.w.process(context.Background(), queuedJob(6, 0, "chan-dest", merge))

	// assert
	require.Len(t, h.fp.calls, 1, "merged notice")
	assert.Equal(t, "🎉 ada merged !12", h.fp.calls[0].message)
	require.Len(t, h.fp.updates, 1, "final root refresh before closing")
	assert.Equal(t, []string{"987"}, h.fs.closed)
	assert.Equal(t, []string{"987"}, h.fs.stateClosed, "board stops push attribution on close")
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
}

func TestWorker_RefreshOnlyDeliverySkipsNotice(t *testing.T) {
	// arrange: pipeline "running" style delivery — board moves, thread stays quiet.
	h := newHarness(t)
	openBoardThread(h)

	// act
	h.w.process(context.Background(), planJob(7, 0, &event.Plan{Refresh: true}))

	// assert
	assert.Empty(t, h.fp.calls)
	require.Len(t, h.fp.updates, 1)
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
}

func TestWorker_MissingBoardSkipsRefreshNotNotice(t *testing.T) {
	// arrange: row from before migration 0002 — no board to render from.
	h := newHarness(t)
	h.fs.thread = &store.Thread{ChannelID: "c", RootPostID: "r", Open: true}
	h.fs.state = nil

	// act
	h.w.process(context.Background(), planJob(8, 0, &event.Plan{Notice: plan.Comment, Refresh: true}))

	// assert: notice posts (it renders from the event), refresh is skipped,
	// the delivery still settles — a retry must not spin on the missing board.
	require.Len(t, h.fp.calls, 1)
	assert.Empty(t, h.fp.updates)
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
}

func TestWorker_BoardStoreFaultRequeuesWithoutBudget(t *testing.T) {
	// arrange: store outage while reading the board (SYSREQ-007 posture).
	h := newHarness(t)
	openBoardThread(h)
	h.fs.errState = errors.New("store down")

	// act
	h.w.process(context.Background(), planJob(9, 4, &event.Plan{Refresh: true}))

	// assert
	s := h.fs.settles[0].s
	require.Equal(t, store.SettlementRetried, s.Kind)
	assert.Equal(t, 4, s.Attempts)
	assert.True(t, s.Next.Equal(workerNow.Add(storeFaultDelay)))
}

func TestWorker_DeletedRootOnRefreshParksAndCloses(t *testing.T) {
	// arrange: root deleted between notice and refresh — SYSREQ-026 covers
	// any root-post operation, update included.
	h := newHarness(t)
	openBoardThread(h)
	h.fp.updateErrs = []error{&mattermost.ThreadGoneError{}}

	// act
	h.w.process(context.Background(), planJob(10, 0, &event.Plan{Refresh: true}))

	// assert
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementParked, h.fs.settles[0].s.Kind)
	assert.Equal(t, metrics.CauseThreadGone, h.fs.settles[0].s.Reason)
	assert.Equal(t, []string{"987"}, h.fs.closed)
}
