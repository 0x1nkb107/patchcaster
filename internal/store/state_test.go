package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0x5a17ed/patchcaster/internal/event"
	"github.com/0x5a17ed/patchcaster/internal/plan"
	"github.com/0x5a17ed/patchcaster/internal/store"
)

// Board semantics under test (SYSREQ-027/030, ADR-014): ingest is the only
// state writer, applying inside the seen-mark makes every effect exactly-once
// per webhook identifier.

func noteEvent(id, action string, system bool) *event.Event {
	ev := &event.Event{
		WebhookID: id, Kind: event.KindNote, Action: action,
		MRRef: "1234", ProjectPath: "grp/app", Actor: "bob",
		NoteCreate: action == "create", System: system,
		When: time.Now().UTC(),
	}
	ev.Plan = plan.For(ev)
	return ev
}

func pipelineEvent(id, status string) *event.Event {
	ev := &event.Event{
		WebhookID: id, Kind: event.KindPipeline, Action: status,
		MRRef: "1234", ProjectPath: "grp/app", PipelineStatus: status,
		When: time.Now().UTC(),
	}
	ev.Plan = plan.For(ev)
	return ev
}

func pushEvent(id string, projectID int64, branch string, commits int) *event.Event {
	ev := &event.Event{
		WebhookID: id, Kind: event.KindPush, ProjectPath: "grp/app", Actor: "ada",
		PushProjectID: projectID, PushBranch: branch, PushCommits: commits,
		When: time.Now().UTC(),
	}
	ev.Plan = plan.For(ev)
	return ev
}

// mrEvent builds a merge request delivery with the given action; mutate adds
// the kind-specific inputs the board consumes.
func mrEvent(id, action string, mutate func(*event.Event)) *event.Event {
	ev := &event.Event{
		WebhookID: id, Kind: event.KindMergeRequest, Action: action,
		MRRef: "1234", IID: 3, ProjectPath: "grp/app", Title: "T",
		Author: "ada", Actor: "ada", URL: "https://x/1",
		SourceBranch: "feat", TargetBranch: "main", SourceProjectID: 42, Open: true,
		When: time.Now().UTC(),
	}
	if mutate != nil {
		mutate(ev)
	}
	ev.Plan = plan.For(ev)
	return ev
}

func TestPostgres_CommentCountIsExactlyOnce(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// a human comment creates counts once and enqueues the notice
	res, err := st.Accept(ctx, noteEvent("msg-c", "create", false), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.True(t, res.Enqueued)

	// the webhook replay changes nothing at all (SYSREQ-027)
	res, err = st.Accept(ctx, noteEvent("msg-c", "create", false), "chan-1", now)
	require.NoError(t, err)
	assert.False(t, res.Accepted)

	// comment edits and system notes move no count and post nothing
	res, err = st.Accept(ctx, noteEvent("msg-c2", "update", false), "chan-1", now)
	require.NoError(t, err)
	assert.True(t, res.Accepted)
	assert.False(t, res.Enqueued)
	res, err = st.Accept(ctx, noteEvent("msg-c3", "create", true), "chan-1", now)
	require.NoError(t, err)
	assert.False(t, res.Enqueued, "system notes never notify")

	board, err := st.State(ctx, "1234")
	require.NoError(t, err)
	assert.Equal(t, 1, board.Comments, "exactly one count for one human comment")
}

func TestPostgres_EditsAndSilentDeliveries(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	_, err := st.Accept(ctx, mrEvent("msg-e0", "open", nil), "chan-1", now)
	require.NoError(t, err)

	// a description change is one edit and silently refreshes (no notice)
	res, err := st.Accept(ctx, mrEvent("msg-e1", "update",
		func(ev *event.Event) { ev.EditChanged = true }), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Enqueued, "a moved board refreshes the root")

	// a label-only update moves no observable field: accepted, unqueued
	res, err = st.Accept(ctx, mrEvent("msg-e2", "update", nil), "chan-1", now)
	require.NoError(t, err)
	assert.True(t, res.Accepted, "the seen mark still commits")
	assert.False(t, res.Enqueued)

	// a title change moves the rendered header: silent refresh (SYSREQ-028)
	res, err = st.Accept(ctx, mrEvent("msg-e3", "update",
		func(ev *event.Event) { ev.Title = "Renamed"; ev.TitleChanged = true }), "chan-1", now)
	require.NoError(t, err)
	assert.True(t, res.Enqueued)

	board, err := st.State(ctx, "1234")
	require.NoError(t, err)
	assert.Equal(t, 1, board.Edits, "only the description change counted")
	assert.Equal(t, "Renamed", board.Title)
}

func TestPostgres_PipelineStatusIsConditionalWrite(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	res, err := st.Accept(ctx, pipelineEvent("msg-p1", "running"), "chan-1", now)
	require.NoError(t, err)
	assert.True(t, res.Enqueued, "first linked event moves the board")

	res, err = st.Accept(ctx, pipelineEvent("msg-p2", "running"), "chan-1", now)
	require.NoError(t, err)
	assert.True(t, res.Accepted)
	assert.False(t, res.Enqueued, "the same status changes nothing: no refresh churn")

	res, err = st.Accept(ctx, pipelineEvent("msg-p3", "success"), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Enqueued)

	board, err := st.State(ctx, "1234")
	require.NoError(t, err)
	assert.Equal(t, "success", board.PipelineStatus)
}

func TestPostgres_PushAttributionAndCountAttach(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	_, err := st.Accept(ctx, mrEvent("msg-a0", "open", nil), "chan-1", now)
	require.NoError(t, err)
	seed, err := st.ClaimDue(ctx, now, time.Minute) // drain the announcement job
	require.NoError(t, err)
	require.NotNil(t, seed)
	require.NoError(t, st.Settle(ctx, seed.ID, now, store.Sent()))

	// the push hook attributes its count to the open request, posts nothing
	res, err := st.Accept(ctx, pushEvent("msg-a1", 42, "feat", 3), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Accepted)
	assert.False(t, res.Enqueued, "pushes never post (SYSREQ-030)")
	assert.Equal(t, 1, res.Attributed)

	// the update-with-push accepted afterwards carries the count
	res, err = st.Accept(ctx, mrEvent("msg-a2", "update",
		func(ev *event.Event) { ev.Pushed = true; ev.EditChanged = true }), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Enqueued)
	job, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	got, err := job.Event()
	require.NoError(t, err)
	require.NotNil(t, got.PushCount, "the notice may state the count (SYSREQ-030)")
	assert.Equal(t, 3, *got.PushCount)
	assert.Equal(t, plan.Push, got.Plan.Notice)

	// a push nobody tracks is a miss, not an error (fork pushes)
	res, err = st.Accept(ctx, pushEvent("msg-a3", 42, "other-branch", 9), "chan-1", now)
	require.NoError(t, err)
	assert.True(t, res.Accepted)
	assert.Equal(t, 0, res.Attributed)

	// closing the request stops attribution: late pushes are misses
	require.NoError(t, st.CloseState(ctx, "1234", now))
	res, err = st.Accept(ctx, pushEvent("msg-a4", 42, "feat", 5), "chan-1", now)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Attributed, "closed merge requests take no pushes")
}

func TestPostgres_PushBeforeUpdateOmitsCount(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	_, err := st.Accept(ctx, mrEvent("msg-r0", "open", nil), "chan-1", now)
	require.NoError(t, err)
	seed, err := st.ClaimDue(ctx, now, time.Minute) // drain the announcement job
	require.NoError(t, err)
	require.NotNil(t, seed)
	require.NoError(t, st.Settle(ctx, seed.ID, now, store.Sent()))

	// the update-with-push lands before its push hook: honest fallback text
	res, err := st.Accept(ctx, mrEvent("msg-r1", "update",
		func(ev *event.Event) { ev.Pushed = true; ev.EditChanged = true }), "chan-1", now)
	require.NoError(t, err)
	require.True(t, res.Enqueued)
	job, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job, "the push notice itself must still deliver")
	got, err := job.Event()
	require.NoError(t, err)
	assert.Nil(t, got.PushCount, "unattributed pushes render without a count")
}

func TestPostgres_ReviewerTallyIsRecomputed(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	_, err := st.Accept(ctx, mrEvent("msg-v0", "update",
		func(ev *event.Event) { ev.ReviewersDone, ev.ReviewersRequired = 2, 3 }), "chan-1", now)
	require.NoError(t, err)

	// an approval-reset push recomputes downward — never a decrement drift.
	// Real unapproved deliveries carry the full payload: done reset, policy
	// unchanged.
	_, err = st.Accept(ctx, mrEvent("msg-v1", "unapproved", func(ev *event.Event) {
		ev.ReviewersDone, ev.ReviewersRequired = 0, 3
	}), "chan-1", now)
	require.NoError(t, err)

	board, err := st.State(ctx, "1234")
	require.NoError(t, err)
	assert.Equal(t, 0, board.ReviewersDone, "tallies are recomputed, not incremented")
	assert.Equal(t, 3, board.ReviewersRequired)
}

func TestPostgres_PhaseFlagsSurviveReclaim(t *testing.T) {
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	_, err := st.Accept(ctx, mrEvent("msg-f1", "open", nil), "chan-1", now)
	require.NoError(t, err)
	job, err := st.ClaimDue(ctx, now, time.Minute)
	require.NoError(t, err)
	require.False(t, job.NoticePosted)
	require.False(t, job.RootRefreshed)

	require.NoError(t, st.MarkNoticePosted(ctx, job.ID, now))
	require.NoError(t, st.ReapLeases(ctx, now.Add(2*time.Minute)))

	reclaimed, err := st.ClaimDue(ctx, now.Add(2*time.Minute), time.Minute)
	require.NoError(t, err)
	require.NotNil(t, reclaimed)
	assert.True(t, reclaimed.NoticePosted, "a requeued delivery must skip its notice phase")
	assert.False(t, reclaimed.RootRefreshed)
}
