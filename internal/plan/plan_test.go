package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/0x5a17ed/patchcaster/internal/event"
)

func TestForDecisionTable(t *testing.T) {
	mr := func(action string, mutate func(*event.Event)) *event.Event {
		ev := &event.Event{Kind: event.KindMergeRequest, Action: action}
		if mutate != nil {
			mutate(ev)
		}
		return ev
	}
	tests := []struct {
		name string
		ev   *event.Event
		want event.Plan
	}{
		{"open announces a root", mr("open", nil), event.Plan{CreateRoot: true}},
		{"close replies and retires", mr("close", nil), event.Plan{Notice: Closed, CloseAfter: true}},
		{"merge celebrates and retires", mr("merge", nil), event.Plan{Notice: Merged, CloseAfter: true}},
		{"approval notifies", mr("approval", nil), event.Plan{Notice: Approval}},
		{"full approval stays silent", mr("approved", nil), event.Plan{}},
		{"reopen stays silent", mr("reopen", nil), event.Plan{}},
		{"approval reset stays silent", mr("unapproved", nil), event.Plan{}},
		{"reviewed transition notifies",
			mr("update", func(ev *event.Event) { ev.ReviewState = event.ReviewReviewed }),
			event.Plan{Notice: Reviewed}},
		{"changes-requested notifies",
			mr("update", func(ev *event.Event) { ev.ReviewState = event.ReviewChanges }),
			event.Plan{Notice: Changes}},
		{"review-with-approval notifies approval",
			mr("update", func(ev *event.Event) { ev.ReviewState = event.ReviewApproved }),
			event.Plan{Notice: Approval}},
		{"verdict outranks accompanying push", mr("update", func(ev *event.Event) {
			ev.ReviewState = event.ReviewReviewed
			ev.Pushed = true
		}), event.Plan{Notice: Reviewed}},
		{"push notifies",
			mr("update", func(ev *event.Event) { ev.Pushed = true }),
			event.Plan{Notice: Push}},
		{"label-only update posts nothing", mr("update", nil), event.Plan{}},
		{"human comment notifies",
			&event.Event{Kind: event.KindNote, Action: "create", NoteCreate: true},
			event.Plan{Notice: Comment}},
		{"edited comment posts nothing",
			&event.Event{Kind: event.KindNote, Action: "update"},
			event.Plan{}},
		{"system note posts nothing",
			&event.Event{Kind: event.KindNote, Action: "create", NoteCreate: true, System: true},
			event.Plan{}},
		{"pipeline success notifies",
			&event.Event{Kind: event.KindPipeline, PipelineStatus: "success"},
			event.Plan{Notice: PipelinePass}},
		{"pipeline failure notifies",
			&event.Event{Kind: event.KindPipeline, PipelineStatus: "failed"},
			event.Plan{Notice: PipelineFail}},
		{"pipeline running stays silent",
			&event.Event{Kind: event.KindPipeline, PipelineStatus: "running"},
			event.Plan{}},
		{"push never posts",
			&event.Event{Kind: event.KindPush, PushBranch: "feat", PushCommits: 3},
			event.Plan{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, *For(tc.ev))
		})
	}
}
