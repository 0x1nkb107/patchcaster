package e2e

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mattermostAPISpec describes the posts endpoints of a pinned Mattermost
// release. testdata/refresh-mattermost-api.sh assembles it from Mattermost's
// own sources (ADR-027).
//
//go:embed testdata/mattermost-openapi-v4-posts.yaml
var mattermostAPISpec []byte

// mattermostAPI checks requests against the published description of
// Mattermost's REST API, so that the fake accepts only what the real server
// would accept.
type mattermostAPI struct {
	router routers.Router
}

// loadMattermostAPI parses the pinned description once for the suite.
var loadMattermostAPI = sync.OnceValues(func() (*mattermostAPI, error) {
	doc, err := openapi3.NewLoader().LoadFromData(mattermostAPISpec)
	if err != nil {
		return nil, fmt.Errorf("parse Mattermost API description: %w", err)
	}
	// The description names Mattermost's own server URL, but the fake
	// listens on a random local port. Matching by path is what matters.
	doc.Servers = openapi3.Servers{{URL: "/"}}

	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("route Mattermost API operations: %w", err)
	}
	return &mattermostAPI{router: router}, nil
})

// check reports how a request departs from the Mattermost API, or nil when
// it is a valid call of a documented operation. Authentication is the fake's
// own concern, so the description's security requirements are not checked
// here.
func (api *mattermostAPI) check(ctx context.Context, r *http.Request) error {
	route, pathParams, err := api.router.FindRoute(r)
	if err != nil {
		return fmt.Errorf("%s %s is no Mattermost API operation: %w", r.Method, r.URL.Path, err)
	}
	input := &openapi3filter.RequestValidationInput{
		Request:    r,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	}
	if err := openapi3filter.ValidateRequest(ctx, input); err != nil {
		return fmt.Errorf("%s request breaks the Mattermost API: %w", route.Operation.OperationID, err)
	}
	return nil
}

// The contract check itself: valid calls pass, and each kind of departure
// from the published API is caught.
func TestMattermostAPICheck(t *testing.T) {
	api, err := loadMattermostAPI()
	require.NoError(t, err)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		valid  bool
	}{
		{"create a post", http.MethodPost, "/api/v4/posts", `{"channel_id":"c1","message":"hello"}`, true},
		{"create a reply", http.MethodPost, "/api/v4/posts", `{"channel_id":"c1","message":"hi","root_id":"p1"}`, true},
		{"create without a channel", http.MethodPost, "/api/v4/posts", `{"message":"hello"}`, false},
		{"create with a misspelled channel field", http.MethodPost, "/api/v4/posts", `{"channel":"c1","message":"hello"}`, false},
		{"create with a message of the wrong type", http.MethodPost, "/api/v4/posts", `{"channel_id":"c1","message":42}`, false},
		{"update a post", http.MethodPut, "/api/v4/posts/p1", `{"id":"p1","message":"edited"}`, true},
		{"update without the post id", http.MethodPut, "/api/v4/posts/p1", `{"message":"edited"}`, false},
		{"call an undocumented endpoint", http.MethodPost, "/api/v4/postings", `{}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+botToken)

			// act
			err := api.check(context.Background(), r)

			// assert
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
