package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The access token the fake GitLab API accepts, and whom it acts as.
const (
	gitlabToken    = "e2e-gitlab-access-token"
	gitlabUsername = "patchcaster-gitlab"
)

// fakeHook is a hook the fake GitLab API holds. Triggers lists the
// triggers it fires for, sorted.
type fakeHook struct {
	ID           int64
	URL          string
	Name         string
	Triggers     []string
	SigningToken string
}

// fakeGitLabAPI answers the GitLab REST API calls that a connection check
// and the hook registration make, for the suite's access token only. It
// keeps hooks by where they live: "system", "groups/<path>" or
// "projects/<path>".
type fakeGitLabAPI struct {
	server *httptest.Server

	mu                sync.Mutex
	hooks             map[string][]fakeHook
	nextID            int64
	groupHooksRefused bool // as on a GitLab without Premium
}

func newFakeGitLabAPI(t *testing.T) *fakeGitLabAPI {
	t.Helper()
	g := &fakeGitLabAPI{hooks: map[string][]fakeHook{}}
	g.server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGitLabAPI) url() string { return g.server.URL }

// refuseGroupHooks makes group hooks fail as GitLab Free fails them.
func (g *fakeGitLabAPI) refuseGroupHooks() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.groupHooksRefused = true
}

// hooksAt returns the hooks at place.
func (g *fakeGitLabAPI) hooksAt(place string) []fakeHook {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.hooks[place])
}

// layout describes every hook as "place: trigger,trigger", sorted, for
// comparing the whole set at once.
func (g *fakeGitLabAPI) layout() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var described []string
	for place, hooks := range g.hooks {
		for _, h := range hooks {
			described = append(described, place+": "+strings.Join(h.Triggers, ","))
		}
	}
	slices.Sort(described)
	return described
}

func writeAPIJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// serve routes by the escaped path, so that "groups/a%2Fb/hooks" names the
// group a/b, as GitLab reads it.
func (g *fakeGitLabAPI) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") != gitlabToken {
		writeAPIJSON(w, http.StatusUnauthorized, map[string]string{"message": "401 Unauthorized"})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), "/api/v4/"), "/")
	if len(parts) == 1 && parts[0] == "user" && r.Method == http.MethodGet {
		writeAPIJSON(w, http.StatusOK, map[string]any{"id": 42, "username": gitlabUsername})
		return
	}
	place, rest, ok := placeOf(parts)
	if !ok {
		writeAPIJSON(w, http.StatusNotFound, map[string]string{"message": "404 Not Found"})
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.HasPrefix(place, "groups/") && g.groupHooksRefused {
		writeAPIJSON(w, http.StatusNotFound, map[string]string{"message": "404 Not Found"})
		return
	}
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		g.list(w, place)
	case len(rest) == 0 && r.Method == http.MethodPost:
		g.create(w, r, place)
	case len(rest) == 1 && r.Method == http.MethodPut:
		g.update(w, r, place, rest[0])
	case len(rest) == 1 && r.Method == http.MethodDelete:
		g.delete(w, place, rest[0])
	default:
		writeAPIJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "405 Method Not Allowed"})
	}
}

// placeOf reads where the hooks of a path live, and what follows "hooks".
func placeOf(parts []string) (string, []string, bool) {
	if len(parts) >= 1 && parts[0] == "hooks" {
		return "system", parts[1:], true
	}
	if len(parts) >= 3 && (parts[0] == "groups" || parts[0] == "projects") && parts[2] == "hooks" {
		path, err := url.PathUnescape(parts[1])
		if err != nil {
			return "", nil, false
		}
		return parts[0] + "/" + path, parts[3:], true
	}
	return "", nil, false
}

// triggersOf reads the triggers a hook request turns on.
func triggersOf(payload map[string]any) []string {
	var on []string
	for _, name := range []string{"merge_requests_events", "push_events", "note_events", "pipeline_events",
		"tag_push_events", "issues_events", "repository_update_events"} {
		if value, _ := payload[name].(bool); value {
			on = append(on, strings.TrimSuffix(name, "_events"))
		}
	}
	slices.Sort(on)
	return on
}

func (g *fakeGitLabAPI) list(w http.ResponseWriter, place string) {
	listed := []map[string]any{}
	for _, h := range g.hooks[place] {
		listed = append(listed, map[string]any{"id": h.ID, "url": h.URL, "name": h.Name})
	}
	writeAPIJSON(w, http.StatusOK, listed)
}

func (g *fakeGitLabAPI) create(w http.ResponseWriter, r *http.Request, place string) {
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAPIJSON(w, http.StatusBadRequest, map[string]string{"message": "400 Bad request"})
		return
	}
	g.nextID++
	hook := fakeHook{ID: g.nextID, Triggers: triggersOf(payload)}
	hook.URL, _ = payload["url"].(string)
	hook.Name, _ = payload["name"].(string)
	hook.SigningToken, _ = payload["signing_token"].(string)
	g.hooks[place] = append(g.hooks[place], hook)
	writeAPIJSON(w, http.StatusCreated, map[string]any{"id": hook.ID, "url": hook.URL, "name": hook.Name})
}

func (g *fakeGitLabAPI) indexOf(place, id string) int {
	return slices.IndexFunc(g.hooks[place], func(h fakeHook) bool { return strconv.FormatInt(h.ID, 10) == id })
}

func (g *fakeGitLabAPI) update(w http.ResponseWriter, r *http.Request, place, id string) {
	index := g.indexOf(place, id)
	if index < 0 {
		writeAPIJSON(w, http.StatusNotFound, map[string]string{"message": "404 Not found"})
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAPIJSON(w, http.StatusBadRequest, map[string]string{"message": "400 Bad request"})
		return
	}
	hook := &g.hooks[place][index]
	hook.Triggers = triggersOf(payload)
	hook.URL, _ = payload["url"].(string)
	writeAPIJSON(w, http.StatusOK, map[string]any{"id": hook.ID, "url": hook.URL, "name": hook.Name})
}

func (g *fakeGitLabAPI) delete(w http.ResponseWriter, place, id string) {
	index := g.indexOf(place, id)
	if index < 0 {
		writeAPIJSON(w, http.StatusNotFound, map[string]string{"message": "404 Not found"})
		return
	}
	g.hooks[place] = slices.Delete(g.hooks[place], index, index+1)
	if len(g.hooks[place]) == 0 {
		delete(g.hooks, place)
	}
	w.WriteHeader(http.StatusNoContent)
}
