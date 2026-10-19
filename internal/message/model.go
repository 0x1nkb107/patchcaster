// Package message writes chat messages from templates over a versioned data
// model (ADR-038). A template is Go's text/template, it writes Markdown, and
// it reads one version of the data model. The built-in templates ship with
// the binary, and a route may replace any of them (SYSREQ-049).
package message

import (
	"strconv"
	"strings"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/markdown"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Version names a version of the data model. A template records the
// version it was written for and keeps reading it after later releases.
type Version int

// Version1 is the first version of the data model.
const Version1 Version = 1

// Known reports whether the data model has version v.
func (v Version) Known() bool {
	return v == Version1
}

// Kind names a message that a template writes: the root of a thread, or
// one kind of notice in it.
type Kind string

// The kinds of messages.
const (
	Root             Kind = "root"
	Comment          Kind = "comment"
	PipelinePassed   Kind = "pipeline_passed"
	PipelineFailed   Kind = "pipeline_failed"
	Reviewed         Kind = "reviewed"
	ChangesRequested Kind = "changes_requested"
	Approved         Kind = "approved"
	Pushed           Kind = "pushed"
	Merged           Kind = "merged"
	Closed           Kind = "closed"
)

// Kinds lists every kind of message, the root first.
var Kinds = []Kind{Root, Comment, PipelinePassed, PipelineFailed, Reviewed, ChangesRequested, Approved, Pushed, Merged, Closed}

// IsNotice reports whether k is a notice, which has an event.
func (k Kind) IsNotice() bool {
	return k != Root
}

// Text is forge text, escaped for Markdown (SYSREQ-051). A template shows
// it as written by printing it, and the code function shows it as a code
// span. Nothing in a template returns it unescaped.
type Text string

// newText escapes forge text.
func newText(raw string) Text {
	return Text(markdown.Escape(raw))
}

// Data is what a template of Version1 reads.
type Data struct {
	// MergeRequest is the merge request the thread follows.
	MergeRequest MergeRequest
	// Status is where the merge request stands.
	Status Status
	// Event is what happened, for a notice. The root's Event is empty.
	Event Event
}

// MergeRequest is the merge request that a thread follows.
type MergeRequest struct {
	// Number is the merge request's number in its project.
	Number int64
	// Title is the merge request's title.
	Title Text
	// Author is the username of the merge request's author.
	Author Text
	// Project is the path of the merge request's project.
	Project Text
	// SourceBranch is the branch the merge request merges.
	SourceBranch Text
	// TargetBranch is the branch it merges into.
	TargetBranch Text
	// URL is the merge request's web address, as a link target for <> or
	// (). It is empty unless the forge gave an http or https address.
	URL string
	// Open is true while the merge request is open.
	Open bool
}

// Status is where a merge request stands.
type Status struct {
	// Pipeline is the state of the latest pipeline: running, passed,
	// failed, waiting, canceled, skipped, or unknown for a state PatchCaster
	// does not know. It is empty before the first pipeline.
	Pipeline string
	// ReviewersDone counts the reviewers who reviewed or approved.
	ReviewersDone int
	// ReviewersRequired counts the reviewers the merge request needs.
	ReviewersRequired int
	// Comments counts the comments.
	Comments int
	// Edits counts the pushes and description changes.
	Edits int
	// Line is the built-in status line, empty when there is no status.
	Line Text
}

// Event is what happened, for a notice.
type Event struct {
	// Kind is the kind of the notice.
	Kind Kind
	// Actor is the username of whoever acted.
	Actor Text
	// Commits counts the commits of a push, 0 when the forge gave no count.
	Commits int
}

// Facts are what a message is about: the merge request's board, and for a
// notice the event. Each version of the data model builds its data from
// them.
type Facts struct {
	Board *store.State
	Event *event.Event
}

// dataV1 builds the Version1 data of a message of kind.
func dataV1(kind Kind, facts Facts) Data {
	board := facts.Board
	data := Data{
		MergeRequest: MergeRequest{
			Number:       board.IID,
			Title:        newText(board.Title),
			Author:       newText(board.Author),
			Project:      newText(board.ProjectPath),
			SourceBranch: newText(board.SourceBranch),
			TargetBranch: newText(board.TargetBranch),
			URL:          markdown.LinkTarget(board.URL),
			Open:         board.Open,
		},
		Status: Status{
			Pipeline:          pipelineState(board.PipelineStatus),
			ReviewersDone:     board.ReviewersDone,
			ReviewersRequired: board.ReviewersRequired,
			Comments:          board.Comments,
			Edits:             board.Edits,
			Line:              newText(statusLine(board)),
		},
	}
	if kind.IsNotice() && facts.Event != nil {
		data.Event = Event{
			Kind:    kind,
			Actor:   newText(facts.Event.Actor),
			Commits: commitCount(facts.Event),
		}
	}
	return data
}

// commitCount returns the attributed commit count of a push, or 0.
func commitCount(ev *event.Event) int {
	if ev.PushCount == nil {
		return 0
	}
	return max(*ev.PushCount, 0)
}

// pipelineState words a GitLab pipeline status in the data model's
// forge-neutral states.
func pipelineState(status string) string {
	switch status {
	case "":
		return ""
	case "running":
		return "running"
	case "success":
		return "passed"
	case "failed":
		return "failed"
	case "created", "pending", "preparing", "waiting_for_resource", "scheduled", "prepared", "waiting", "manual":
		return "waiting"
	case "canceled", "canceling":
		return "canceled"
	case "skipped":
		return "skipped"
	default:
		return "unknown"
	}
}

// statusLine assembles the root's status line (SYSREQ-028): segments
// joined by " • ", each present only when its signal exists, and empty when
// none is.
func statusLine(board *store.State) string {
	var segments []string
	if board.PipelineStatus != "" {
		segments = append(segments, pipelineSegment(board.PipelineStatus))
	}
	if board.ReviewersRequired > 0 {
		segments = append(segments, "👀 "+strconv.Itoa(board.ReviewersDone)+"/"+strconv.Itoa(board.ReviewersRequired)+" reviewers")
	}
	if board.Comments > 0 {
		segments = append(segments, "💬 "+countOf(board.Comments, "comment", "comments"))
	}
	if board.Edits > 0 {
		segments = append(segments, "📝 "+countOf(board.Edits, "edit", "edits"))
	}
	return strings.Join(segments, " • ")
}

// pipelineSegment maps GitLab pipeline statuses onto the owner's fixed
// emoji table (2026-10-10). An unknown status shows as it is, with a
// neutral marker, and never fails the render.
func pipelineSegment(status string) string {
	switch status {
	case "running":
		return "🟡 Pipeline running"
	case "success":
		return "🟢 Pipeline passed"
	case "failed":
		return "🔴 Pipeline failed"
	case "waiting_for_resource", "scheduled", "prepared", "waiting", "manual":
		return "⚪ Pipeline waiting"
	case "canceled":
		return "⚪ Pipeline canceled"
	case "skipped":
		return "⚪ Pipeline skipped"
	default:
		return "⚪ Pipeline " + status
	}
}

// countOf words a count with the singular noun at one and the plural
// otherwise, as "1 comment" or "3 comments".
func countOf(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}
