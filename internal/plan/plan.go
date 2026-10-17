// Package plan is the single delivery-policy table (ADR-014): what an
// accepted delivery posts. Ingest consults it to decide what to enqueue; the
// worker re-derives from it only for rows persisted before plans existed.
// Policy lives here, not at either edge, so the two can never drift.
package plan

import (
	"slices"

	"github.com/patchcaster-app/patchcaster/internal/event"
)

// Notice template names: each maps to templates/notice_<name>.tmpl in the
// renderer, and the set is exactly the notifying event kinds of SYSREQ-029
// plus the legacy close/merge replies (SYSREQ-008).
const (
	Comment      = "comment"
	PipelinePass = "pipeline_pass"
	PipelineFail = "pipeline_fail"
	Reviewed     = "reviewed"
	Changes      = "changes"
	Approval     = "approval"
	Push         = "push"
	Merged       = "merged"
	Closed       = "closed"
)

// For decides the delivery plan for an accepted event. An empty notice means
// the delivery touches the board only; Plan.Refresh is left to the store,
// which knows what actually changed (SYSREQ-028).
func For(ev *event.Event) *event.Plan {
	switch ev.Kind {
	case event.KindMergeRequest:
		return forMergeRequest(ev)
	case event.KindNote:
		if ev.NoteCreate && !ev.System {
			return &event.Plan{Notice: Comment}
		}
		return &event.Plan{}
	case event.KindPipeline:
		switch ev.PipelineStatus {
		case "success":
			return &event.Plan{Notice: PipelinePass}
		case "failed":
			return &event.Plan{Notice: PipelineFail}
		}
		return &event.Plan{}
	default: // KindPush: attribution only, never posts (SYSREQ-030).
		return &event.Plan{}
	}
}

func forMergeRequest(ev *event.Event) *event.Plan {
	switch ev.Action {
	case "open":
		return &event.Plan{CreateRoot: true}
	case "close":
		return &event.Plan{Notice: Closed, CloseAfter: true}
	case "merge":
		return &event.Plan{Notice: Merged, CloseAfter: true}
	case "approval":
		return &event.Plan{Notice: Approval}
	case "update":
		return forUpdate(ev)
	default: // reopen, unapproval, approved, ...: state only (SYSREQ-029).
		return &event.Plan{}
	}
}

func forUpdate(ev *event.Event) *event.Plan {
	// Review verdicts outrank the push they may accompany; a plain update
	// (labels, title, ...) posts nothing (SYSREQ-029).
	switch ev.ReviewState {
	case event.ReviewReviewed:
		return &event.Plan{Notice: Reviewed}
	case event.ReviewChanges:
		return &event.Plan{Notice: Changes}
	case event.ReviewApproved:
		return &event.Plan{Notice: Approval}
	}
	if ev.Pushed {
		return &event.Plan{Notice: Push}
	}
	return &event.Plan{}
}

// The reply kinds a route may mute (SYSREQ-047). The management API's
// description names the same ones.
const (
	ReplyComment  = "comment"
	ReplyReview   = "review"
	ReplyPipeline = "pipeline"
	ReplyPush     = "push"
	ReplyMerge    = "merge"
	ReplyClose    = "close"
)

// ReplyKind names the kind of a notice reply, by which a route mutes it. A
// plan without a notice has none.
func ReplyKind(notice string) string {
	switch notice {
	case Comment:
		return ReplyComment
	case Reviewed, Changes, Approval:
		return ReplyReview
	case PipelinePass, PipelineFail:
		return ReplyPipeline
	case Push:
		return ReplyPush
	case Merged:
		return ReplyMerge
	case Closed:
		return ReplyClose
	}
	return ""
}

// ForSink returns the plan of one sink: p without its reply when the sink's
// routes mute the reply's kind. The root, the board refresh and the closing
// stay whatever a route mutes, so every sink shows the same board
// (SYSREQ-047).
func ForSink(p *event.Plan, muted []string) *event.Plan {
	sinkPlan := *p
	if p.Notice != "" && slices.Contains(muted, ReplyKind(p.Notice)) {
		sinkPlan.Notice = ""
	}
	return &sinkPlan
}
