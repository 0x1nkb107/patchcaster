package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The access token the fake GitLab API accepts, and whom it acts as.
const (
	gitlabToken    = "e2e-gitlab-access-token"
	gitlabUsername = "patchcaster-gitlab"
)

// fakeGitLabAPI answers the GitLab REST API calls a connection check makes,
// for the suite's access token only.
type fakeGitLabAPI struct {
	server *httptest.Server
}

func newFakeGitLabAPI(t *testing.T) *fakeGitLabAPI {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("PRIVATE-TOKEN") != gitlabToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"username":"` + gitlabUsername + `"}`))
	})
	g := &fakeGitLabAPI{server: httptest.NewServer(mux)}
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGitLabAPI) url() string { return g.server.URL }
