// Package event parses GitLab merge request webhook payloads into a normalized
// model (SYSREQ-001). It is the only place that speaks GitLab's payload dialect;
// everything downstream consumes Event.
package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// KindMergeRequest is the object_kind value of merge request payloads.
const KindMergeRequest = "merge_request"

// Event is the normalized merge request event.
//
// Identity is object_attributes.id — GitLab's globally unique merge request id.
// This is the SWDD choice for "merge request identity" (SYSREQ-006): it is
// stable, never reused, and present in every payload scope.
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
}

// Forwardable reports whether Kind is a merge request event.
func (e *Event) Forwardable() bool { return e.Kind == KindMergeRequest }

// raw is the wire shape. Pointers on timestamps distinguish absent from empty.
type raw struct {
	ObjectKind string `json:"object_kind"`
	EventName  string `json:"event_name"`
	User       struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"user"`
	Project struct {
		ID                int64  `json:"id"`
		FullPath          string `json:"full_path"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
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
	} `json:"object_attributes"`
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
	e := &Event{Kind: r.ObjectKind, Actor: r.User.Username}
	if e.Kind != KindMergeRequest {
		return e, nil // kinds we do not forward need no MR-specific validation
	}
	a := &r.ObjectAttributes
	if a.Action == "" {
		return nil, fmt.Errorf("%w: missing object_attributes.action", ErrInvalidPayload)
	}
	path := r.Project.FullPath
	if path == "" {
		path = r.Project.PathWithNamespace
	}
	if path == "" {
		return nil, fmt.Errorf("%w: missing project path", ErrInvalidPayload)
	}
	e.Action = a.Action
	e.MRRef = fmt.Sprintf("%d", a.ID)
	e.IID = a.IID
	e.ProjectPath = path
	e.Title = a.Title
	e.URL = a.URL
	if a.Author != nil && a.Author.Username != "" {
		e.Author = a.Author.Username
	} else {
		e.Author = r.User.Username
	}
	if e.Actor == "" {
		e.Actor = e.Author
	}
	e.When = parseTimestamp(a.Timestamp, a.UpdatedAt, a.CreatedAt)
	return e, nil
}

// parseTimestamp reads GitLab 19 ISO 8601 values (millisecond precision,
// zone designator) with a fallback chain; the pre-19.0 format is deliberately
// not accepted (solution constraint).
func parseTimestamp(fields ...*string) time.Time {
	for _, f := range fields {
		if f == nil || *f == "" {
			continue
		}
		s := strings.TrimSpace(*f)
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
