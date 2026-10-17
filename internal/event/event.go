// Package event parses GitLab webhook payloads into a normalized model
// (SYSREQ-001). It is the only place that speaks GitLab's payload dialect;
// everything downstream consumes Event. Four kinds are processed — merge
// request, comment (note), pipeline, and push (SYSREQ-008); every other kind
// parses to a bare Event and is acknowledged without effect.
package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Processed webhook kinds (SYSREQ-008's kind gate).
const (
	KindMergeRequest = "merge_request"
	KindNote         = "note"
	KindPipeline     = "pipeline"
	KindPush         = "push"
)

// Submitted reviewer states (SYSREQ-027's numerator members), mirrored from
// GitLab's per-reviewer state values.
const (
	ReviewReviewed = "reviewed"
	ReviewChanges  = "requested_changes"
	ReviewApproved = "approved"
)

// Event is the normalized webhook event.
//
// Identity is object_attributes.id — GitLab's globally unique merge request
// id. This is the SWDD choice for "merge request identity" (SYSREQ-006): it is
// stable, never reused, and present in every merge request payload; note and
// pipeline deliveries name the same id through their linkage object.
//
// The json tags are storage keys: the normalized event is what the queue
// persists, so they must not drift.
type Event struct {
	WebhookID   string    `json:"webhook_id"`   // identity used for dedupe (SYSREQ-014)
	Kind        string    `json:"kind"`         // object_kind
	Action      string    `json:"action"`       // object_attributes.action
	MRRef       string    `json:"mr_ref"`       // string(object_attributes.id), store key
	IID         int64     `json:"iid"`          // per-project merge request number
	ProjectPath string    `json:"project_path"` // routing key (SYSREQ-009)
	Title       string    `json:"title"`        // untrusted; escape before rendering (SYSREQ-003)
	Author      string    `json:"author"`       // merge request author username
	Actor       string    `json:"actor"`        // user who triggered this event
	URL         string    `json:"url"`          // web url of the merge request
	When        time.Time `json:"when"`         // event time from the signed body (SYSREQ-013)

	// Derived-state inputs (SYSREQ-027), populated per kind.
	Open              bool   `json:"open,omitempty"`          // merge request state is "opened"
	SourceBranch      string `json:"source_branch,omitempty"` // push attribution key (SYSREQ-030)
	TargetBranch      string `json:"target_branch,omitempty"` // root header line (SYSREQ-003)
	SourceProjectID   int64  `json:"source_project_id,omitempty"`
	ReviewersDone     int    `json:"reviewers_done,omitempty"`     // state reviewed or approved
	ReviewersRequired int    `json:"reviewers_required,omitempty"` // approval-rule minimum, else assigned count
	Pushed            bool   `json:"pushed,omitempty"`             // update with oldrev: source branch gained commits
	EditChanged       bool   `json:"edit_changed,omitempty"`       // counts as one edit (push or description)
	TitleChanged      bool   `json:"title_changed,omitempty"`
	System            bool   `json:"system,omitempty"`       // system-initiated delivery, never notifies
	ReviewState       string `json:"review_state,omitempty"` // acting reviewer's submitted state, if this event transitioned it (SYSREQ-029)
	NoteCreate        bool   `json:"note_create,omitempty"`  // note action "create"
	PipelineStatus    string `json:"pipeline_status,omitempty"`
	PushProjectID     int64  `json:"push_project_id,omitempty"`
	PushBranch        string `json:"push_branch,omitempty"`
	PushCommits       int    `json:"push_commits,omitempty"` // total_commits_count

	// Filled by ingest when enqueueing (not by the wire):
	ConnectionID int64 `json:"connection_id,omitempty"` // forge connection the delivery came through
	PushCount    *int  `json:"push_count,omitempty"`    // attributed commit count for a push notice (SYSREQ-030)
	Plan         *Plan `json:"plan,omitempty"`          // delivery plan decided at acceptance, for one sink (ADR-014)
}

// Plan is the delivery instruction set decided at acceptance, so the worker
// executes instead of re-interpreting GitLab payloads (ADR-014). Phases run
// in order: notice reply, then root refresh; CloseAfter settles last.
type Plan struct {
	CreateRoot bool   `json:"create_root,omitempty"` // announce: root post + mapping
	Notice     string `json:"notice,omitempty"`      // reply template name; empty posts no reply
	Refresh    bool   `json:"refresh,omitempty"`     // PUT the root status line
	CloseAfter bool   `json:"close_after,omitempty"` // close the mapping once delivered
}

// Forwardable reports whether Kind is a merge request event.
func (e *Event) Forwardable() bool { return e.Kind == KindMergeRequest }

// Processed reports whether the delivery names a kind the service handles at
// all — anything else is acknowledged and discarded (SYSREQ-008).
func (e *Event) Processed() bool {
	switch e.Kind {
	case KindMergeRequest, KindNote, KindPipeline, KindPush:
		return true
	}
	return false
}

// raw is the wire shape: a union of the fields the processed kinds carry.
// Pointers on timestamps distinguish absent from empty.
type raw struct {
	ObjectKind string `json:"object_kind"`
	EventName  string `json:"event_name"`
	User       struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"user"`
	Project struct {
		ID                int64  `json:"id"`
		FullPath          string `json:"full_path"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	ObjectAttributes struct {
		ID        int64   `json:"id"`
		IID       int64   `json:"iid"`
		Title     string  `json:"title"`
		URL       string  `json:"url"`
		Action    string  `json:"action"`
		Timestamp *string `json:"timestamp"`
		UpdatedAt *string `json:"updated_at"`
		CreatedAt *string `json:"created_at"`
		Author    *struct {
			Username string `json:"username"`
		} `json:"author"`

		// merge request state inputs
		State           string `json:"state"`
		SourceBranch    string `json:"source_branch"`
		TargetBranch    string `json:"target_branch"`
		SourceProjectID int64  `json:"source_project_id"`
		Oldrev          string `json:"oldrev"`
		System          bool   `json:"system"`
		ApprovalRules   []struct {
			ApprovalsRequired int `json:"approvals_required"`
		} `json:"approval_rules"`

		// note inputs
		NoteableType string `json:"noteable_type"`

		// pipeline inputs
		Status     string  `json:"status"`
		FinishedAt *string `json:"finished_at"`

		// push inputs (top level for pushes; see PushHook payload shape)
		Ref string `json:"ref"`
	} `json:"object_attributes"`
	Reviewers []struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
	} `json:"reviewers"`
	Changes struct {
		Description json.RawMessage `json:"description"`
		Title       json.RawMessage `json:"title"`
		Reviewers   reviewerChange  `json:"reviewers"`
	} `json:"changes"`

	// note / pipeline merge request linkage
	MergeRequest struct {
		ID  int64 `json:"id"`
		IID int64 `json:"iid"`
	} `json:"merge_request"`

	// push payload fields (top level, not under object_attributes)
	Ref               string `json:"ref"`
	TotalCommitsCount int    `json:"total_commits_count"`
	Commits           []struct {
		Timestamp string `json:"timestamp"`
	} `json:"commits"`
	UserName     string `json:"user_name"`
	UserUsername string `json:"user_username"`
}

// reviewerChange models changes.reviewers. GitLab's public docs render it as
// a positional array of arrays ([[previous], [current]]), but captured real
// payloads — and GitLab's own Go client — send a {previous, current} object.
// Both spellings decode here; betting on one would silently drop every
// review notice whenever GitLab's docs and reality disagree (SYSREQ-029).
// Source: https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2984
type reviewerChange struct {
	Previous []reviewerSnapshot
	Current  []reviewerSnapshot
}

type reviewerSnapshot struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
}

func (r *reviewerChange) UnmarshalJSON(b []byte) error {
	var obj struct {
		Previous []reviewerSnapshot `json:"previous"`
		Current  []reviewerSnapshot `json:"current"`
	}
	if err := json.Unmarshal(b, &obj); err == nil {
		r.Previous, r.Current = obj.Previous, obj.Current
		return nil
	}
	var pair [][]reviewerSnapshot
	if err := json.Unmarshal(b, &pair); err != nil {
		return fmt.Errorf("changes.reviewers: %w", err)
	}
	if len(pair) == 2 {
		r.Previous, r.Current = pair[0], pair[1]
	}
	return nil
}

// ErrInvalidPayload marks payloads that must be answered 4xx without posting
// (SYSREQ-001 acceptance criteria).
var ErrInvalidPayload = errors.New("invalid merge request payload")

// Parse normalizes a verified webhook body. A body whose JSON is valid but
// whose event timestamp is missing or unparseable parses successfully with a
// zero When; the freshness cap treats those as expired (SYSREQ-013).
func Parse(body []byte) (*Event, error) {
	var r raw
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}
	if r.ObjectKind == "" {
		return nil, fmt.Errorf("%w: missing object_kind", ErrInvalidPayload)
	}
	switch r.ObjectKind {
	case KindMergeRequest:
		return parseMergeRequest(&r)
	case KindNote:
		return parseNote(&r)
	case KindPipeline:
		return parsePipeline(&r)
	case KindPush:
		return parsePush(&r)
	default:
		return &Event{Kind: r.ObjectKind}, nil // unhandled kind: acknowledged, inert
	}
}

func projectPath(r *raw) string {
	if r.Project.FullPath != "" {
		return r.Project.FullPath
	}
	return r.Project.PathWithNamespace
}

func parseMergeRequest(r *raw) (*Event, error) {
	a := &r.ObjectAttributes
	if a.Action == "" {
		return nil, fmt.Errorf("%w: missing object_attributes.action", ErrInvalidPayload)
	}
	path := projectPath(r)
	if path == "" {
		return nil, fmt.Errorf("%w: missing project path", ErrInvalidPayload)
	}
	e := &Event{
		Kind:            KindMergeRequest,
		Actor:           r.User.Username,
		Action:          a.Action,
		MRRef:           fmt.Sprintf("%d", a.ID),
		IID:             a.IID,
		ProjectPath:     path,
		Title:           a.Title,
		URL:             a.URL,
		Open:            a.State == "opened",
		SourceBranch:    a.SourceBranch,
		TargetBranch:    a.TargetBranch,
		SourceProjectID: a.SourceProjectID,
		System:          a.System,
		Pushed:          a.Oldrev != "",
	}
	if a.Author != nil && a.Author.Username != "" {
		e.Author = a.Author.Username
	} else {
		e.Author = r.User.Username
	}
	if e.Actor == "" {
		e.Actor = e.Author
	}
	e.ReviewersDone, e.ReviewersRequired = reviewerTally(r)
	e.EditChanged = a.Oldrev != "" || r.Changes.Description != nil
	e.TitleChanged = r.Changes.Title != nil
	e.ReviewState = reviewTransition(r)
	e.When = parseTimestamp(a.Timestamp, a.UpdatedAt, a.CreatedAt)
	return e, nil
}

// reviewerTally derives the board's `n/m reviewers` counts (SYSREQ-027):
// n = assigned reviewers in a submitted state; m = the approval-rule minimum
// when it is positive, else the number of assigned reviewers.
func reviewerTally(r *raw) (done, required int) {
	for _, rev := range r.Reviewers {
		switch rev.State {
		case ReviewReviewed, ReviewApproved:
			done++
		}
	}
	for _, rule := range r.ObjectAttributes.ApprovalRules {
		required += rule.ApprovalsRequired
	}
	if required <= 0 {
		required = len(r.Reviewers)
	}
	return done, required
}

// reviewTransition reports a submitted-review state change for the acting
// user inside changes.reviewers (SYSREQ-029). Re-requests (re-requested
// reviewers move back to unreviewed) and unsubmitted states are not notices.
func reviewTransition(r *raw) string {
	prevState := map[int64]string{}
	for _, rev := range r.Changes.Reviewers.Previous {
		prevState[rev.ID] = rev.State
	}
	for _, rev := range r.Changes.Reviewers.Current {
		if rev.ID != r.User.ID {
			continue
		}
		switch rev.State {
		case ReviewReviewed, ReviewChanges, ReviewApproved:
			if prevState[rev.ID] != rev.State {
				return rev.State
			}
		}
	}
	return ""
}

func parseNote(r *raw) (*Event, error) {
	a := &r.ObjectAttributes
	e := &Event{
		Kind:        KindNote,
		Actor:       r.User.Username,
		Action:      a.Action,
		ProjectPath: projectPath(r),
		System:      a.System,
		NoteCreate:  a.Action == "create",
	}
	if a.NoteableType == "MergeRequest" && r.MergeRequest.ID != 0 {
		e.MRRef = fmt.Sprintf("%d", r.MergeRequest.ID)
		e.IID = r.MergeRequest.IID
	}
	e.When = parseTimestamp(a.CreatedAt, a.UpdatedAt)
	return e, nil
}

func parsePipeline(r *raw) (*Event, error) {
	a := &r.ObjectAttributes
	e := &Event{
		Kind:           KindPipeline,
		Actor:          r.User.Username,
		Action:         a.Status,
		ProjectPath:    projectPath(r),
		PipelineStatus: a.Status,
	}
	if r.MergeRequest.ID != 0 {
		e.MRRef = fmt.Sprintf("%d", r.MergeRequest.ID)
		e.IID = r.MergeRequest.IID
	}
	e.When = parseTimestamp(a.FinishedAt, a.CreatedAt, a.Timestamp, a.UpdatedAt)
	return e, nil
}

func parsePush(r *raw) (*Event, error) {
	e := &Event{
		Kind:          KindPush,
		Actor:         firstNonEmpty(r.UserUsername, r.User.Username, r.User.Name, r.UserName),
		ProjectPath:   firstNonEmpty(projectPath(r), r.Repository.FullName),
		PushProjectID: r.Project.ID,
		PushCommits:   r.TotalCommitsCount,
	}
	if ref := r.Ref; strings.HasPrefix(ref, "refs/heads/") {
		e.PushBranch = strings.TrimPrefix(ref, "refs/heads/")
	}
	// Push payloads timestamp per commit, newest last; branch creations carry
	// no commits and keep a zero When, which the freshness cap discards — an
	// empty branch push has no count to report anyway.
	if n := len(r.Commits); n > 0 {
		if ts := parseTimestampString(r.Commits[n-1].Timestamp); !ts.IsZero() {
			e.When = ts
		}
	}
	return e, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseTimestamp reads GitLab 19 ISO 8601 values (millisecond precision,
// zone designator) with a fallback chain; the pre-19.0 format is deliberately
// not accepted (solution constraint).
func parseTimestamp(fields ...*string) time.Time {
	for _, f := range fields {
		if f == nil {
			continue
		}
		if t := parseTimestampString(*f); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func parseTimestampString(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
