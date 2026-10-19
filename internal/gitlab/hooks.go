package gitlab

import (
	"context"
	"errors"
	"net/http"
	"strconv"
)

// HookKind names where a hook lives.
type HookKind string

// The kinds of hooks. A system hook needs a GitLab administrator's
// token, a group hook GitLab Premium and the group's Owner role, and a
// project hook the project's Maintainer role.
const (
	SystemHook  HookKind = "system"
	GroupHook   HookKind = "group"
	ProjectHook HookKind = "project"
)

// HookScope names one place for hooks: the instance, or a group or project
// by its full path.
type HookScope struct {
	Kind HookKind
	Path string
}

// HookEvents are the triggers a hook fires for. A system hook knows only
// merge requests and pushes.
type HookEvents struct {
	MergeRequests bool
	Pushes        bool
	Comments      bool
	Pipelines     bool
}

// HookSpec is a hook as PatchCaster registers it. SigningToken is sent on
// creation only, and GitLab never shows it again.
type HookSpec struct {
	URL          string
	Name         string
	Description  string
	Events       HookEvents
	SigningToken string
}

// Hook is a hook as GitLab lists it.
type Hook struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// hooksPath returns the path segments of the hooks of scope.
func hooksPath(scope HookScope) []string {
	switch scope.Kind {
	case SystemHook:
		return []string{"hooks"}
	case GroupHook:
		return []string{"groups", scope.Path, "hooks"}
	default:
		return []string{"projects", scope.Path, "hooks"}
	}
}

// hookPath returns the path segments of one hook of scope.
func hookPath(scope HookScope, id int64) []string {
	return append(hooksPath(scope), strconv.FormatInt(id, 10))
}

// payloadOf builds the request body of spec for a hook of kind. Every
// trigger is set, so that GitLab's defaults, which turn pushes and
// repository updates on, never add events PatchCaster did not ask for.
func payloadOf(kind HookKind, spec HookSpec) map[string]any {
	payload := map[string]any{
		"url":                     spec.URL,
		"name":                    spec.Name,
		"description":             spec.Description,
		"enable_ssl_verification": true,
		"merge_requests_events":   spec.Events.MergeRequests,
		"push_events":             spec.Events.Pushes,
		"tag_push_events":         false,
	}
	if kind == SystemHook {
		payload["repository_update_events"] = false
	} else {
		payload["note_events"] = spec.Events.Comments
		payload["pipeline_events"] = spec.Events.Pipelines
		payload["issues_events"] = false
	}
	if spec.SigningToken != "" {
		payload["signing_token"] = spec.SigningToken
	}
	return payload
}

// Hooks lists the hooks of scope.
func (c *Client) Hooks(ctx context.Context, scope HookScope) ([]Hook, error) {
	var hooks []Hook
	for page := 1; page != 0; {
		endpoint := c.endpoint(hooksPath(scope)...) + "?per_page=100&page=" + strconv.Itoa(page)
		a, err := c.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		var batch []Hook
		if err := a.expect(http.StatusOK, &batch); err != nil {
			return nil, err
		}
		hooks = append(hooks, batch...)
		page = a.nextPage()
	}
	return hooks, nil
}

// CreateHook registers a hook in scope and returns its id.
func (c *Client) CreateHook(ctx context.Context, scope HookScope, spec HookSpec) (int64, error) {
	a, err := c.do(ctx, http.MethodPost, c.endpoint(hooksPath(scope)...), payloadOf(scope.Kind, spec))
	if err != nil {
		return 0, err
	}
	var created Hook
	if err := a.expect(http.StatusCreated, &created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

// UpdateHook changes the address, name and triggers of the hook with id in
// scope. Its signing token stays.
func (c *Client) UpdateHook(ctx context.Context, scope HookScope, id int64, spec HookSpec) error {
	spec.SigningToken = ""
	a, err := c.do(ctx, http.MethodPut, c.endpoint(hookPath(scope, id)...), payloadOf(scope.Kind, spec))
	if err != nil {
		return err
	}
	return a.expect(http.StatusOK, nil)
}

// DeleteHook removes the hook with id from scope.
func (c *Client) DeleteHook(ctx context.Context, scope HookScope, id int64) error {
	a, err := c.do(ctx, http.MethodDelete, c.endpoint(hookPath(scope, id)...), nil)
	if err != nil {
		return err
	}
	if a.status == http.StatusOK {
		return nil
	}
	return a.expect(http.StatusNoContent, nil)
}

// IsNotFound reports whether err is GitLab's answer that a resource does
// not exist, such as a hook removed by hand.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}
