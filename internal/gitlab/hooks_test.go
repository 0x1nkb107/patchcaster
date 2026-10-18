package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorded is one request the fake GitLab received.
type recorded struct {
	method  string
	path    string
	token   string
	payload map[string]any
}

func newTestClient(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*Client, *[]recorded) {
	t.Helper()
	var requests []recorded
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recorded{method: r.Method, path: r.URL.EscapedPath(), token: r.Header.Get("PRIVATE-TOKEN")}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&rec.payload)
		}
		requests = append(requests, rec)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL + "/gitlab/")
	require.NoError(t, err)
	return New(base, "access-token"), &requests
}

var fullEvents = HookEvents{MergeRequests: true, Pushes: true, Comments: true, Pipelines: true}

func TestAHookIsCreatedWhereItsScopeLives(t *testing.T) {
	tests := []struct {
		name     string
		scope    HookScope
		wantPath string
		wantKeys []string
		denyKeys []string
	}{
		{
			name:     "a project by its path",
			scope:    HookScope{Kind: ProjectHook, Path: "payments/api"},
			wantPath: "/gitlab/api/v4/projects/payments%2Fapi/hooks",
			wantKeys: []string{"note_events", "pipeline_events"},
			denyKeys: []string{"repository_update_events"},
		},
		{
			name:     "a group by its path",
			scope:    HookScope{Kind: GroupHook, Path: "payments"},
			wantPath: "/gitlab/api/v4/groups/payments/hooks",
			wantKeys: []string{"note_events", "pipeline_events"},
		},
		{
			name:     "the instance",
			scope:    HookScope{Kind: SystemHook},
			wantPath: "/gitlab/api/v4/hooks",
			wantKeys: []string{"repository_update_events"},
			denyKeys: []string{"note_events", "pipeline_events"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			client, requests := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":17}`))
			})

			// act
			id, err := client.CreateHook(context.Background(), tc.scope, HookSpec{
				URL:          "https://hooks.example.org/webhooks/3",
				Name:         "PatchCaster",
				Events:       fullEvents,
				SigningToken: "whsec_key",
			})

			// assert
			require.NoError(t, err)
			assert.Equal(t, int64(17), id)
			require.Len(t, *requests, 1)
			got := (*requests)[0]
			assert.Equal(t, http.MethodPost, got.method)
			assert.Equal(t, tc.wantPath, got.path)
			assert.Equal(t, "access-token", got.token)
			assert.Equal(t, "whsec_key", got.payload["signing_token"])
			assert.Equal(t, false, got.payload["tag_push_events"], "unused triggers are off, not left to GitLab's defaults")
			for _, key := range tc.wantKeys {
				assert.Contains(t, got.payload, key)
			}
			for _, key := range tc.denyKeys {
				assert.NotContains(t, got.payload, key)
			}
		})
	}
}

func TestAnUpdateKeepsTheSigningToken(t *testing.T) {
	// arrange
	client, requests := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":17}`))
	})

	// act
	err := client.UpdateHook(context.Background(), HookScope{Kind: GroupHook, Path: "payments"}, 17, HookSpec{
		URL:          "https://hooks.example.org/webhooks/3",
		Events:       HookEvents{Comments: true, Pipelines: true},
		SigningToken: "whsec_never_sent",
	})

	// assert
	require.NoError(t, err)
	got := (*requests)[0]
	assert.Equal(t, http.MethodPut, got.method)
	assert.Equal(t, "/gitlab/api/v4/groups/payments/hooks/17", got.path)
	assert.NotContains(t, got.payload, "signing_token")
	assert.Equal(t, false, got.payload["merge_requests_events"])
	assert.Equal(t, true, got.payload["note_events"])
}

func TestHooksAreListedPageByPage(t *testing.T) {
	// arrange
	client, requests := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"id":1,"url":"https://a.example","name":"one"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":2,"url":"https://b.example","name":"two"}]`))
	})

	// act
	hooks, err := client.Hooks(context.Background(), HookScope{Kind: ProjectHook, Path: "payments/api"})

	// assert
	require.NoError(t, err)
	assert.Equal(t, []Hook{{ID: 1, URL: "https://a.example", Name: "one"}, {ID: 2, URL: "https://b.example", Name: "two"}}, hooks)
	assert.Len(t, *requests, 2)
}

func TestARefusalCarriesGitLabsAnswer(t *testing.T) {
	// arrange
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Group Not Found"}`))
	})

	// act
	_, createErr := client.CreateHook(context.Background(), HookScope{Kind: GroupHook, Path: "payments"}, HookSpec{URL: "https://x"})
	deleteErr := client.DeleteHook(context.Background(), HookScope{Kind: GroupHook, Path: "payments"}, 5)

	// assert
	require.Error(t, createErr)
	assert.Equal(t, "gitlab answered http 404: 404 Group Not Found", createErr.Error())
	assert.True(t, IsNotFound(deleteErr), "a hook removed by hand answers 404")
}

func TestADeletedHookAnswersNoContent(t *testing.T) {
	// arrange
	client, requests := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// act
	err := client.DeleteHook(context.Background(), HookScope{Kind: SystemHook}, 9)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "/gitlab/api/v4/hooks/9", (*requests)[0].path)
}
