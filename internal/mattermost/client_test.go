package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New(srv.URL, "bot-token")
	return c, srv.Close
}

func TestPost_SuccessSendsAuthAndBody(t *testing.T) {
	// arrange
	var gotAuth, gotCT string
	var gotBody map[string]any
	c, stop := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"post-77"}`))
	})
	defer stop()

	// act
	id, err := c.Post(context.Background(), "chan-1", "hello", "root-9")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "post-77", id)
	assert.Equal(t, "Bearer bot-token", gotAuth)
	assert.Equal(t, "application/json", gotCT)
	assert.Equal(t, "chan-1", gotBody["channel_id"])
	assert.Equal(t, "hello", gotBody["message"])
	assert.Equal(t, "root-9", gotBody["root_id"])
}

func TestPost_RootOmittedWhenEmpty(t *testing.T) {
	// arrange
	var raw string
	c, stop := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		raw = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p"}`))
	})
	defer stop()

	// act
	_, err := c.Post(context.Background(), "c", "m", "")

	// assert
	require.NoError(t, err)
	assert.NotContains(t, raw, "root_id", "root posts must not carry an empty root_id")
}

func TestPost_ErrorTaxonomy(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		headers    map[string]string
		wantKind   string // gone | ratelimited | api
		wantStatus int
	}{
		{
			name:     "429 with reset",
			status:   http.StatusTooManyRequests,
			body:     `{"id":"api.rate_limit"}`,
			headers:  map[string]string{"RateLimit-Reset": "42"},
			wantKind: "ratelimited",
		},
		{
			name:     "404 missing root",
			status:   http.StatusNotFound,
			body:     `{"id":"x"}`,
			wantKind: "gone",
		},
		{
			name:     "410 deleted root",
			status:   http.StatusGone,
			body:     `{"id":"model.post.post_not_found.app_default"}`,
			wantKind: "gone",
		},
		{
			name:     "400 revealing deleted root id",
			status:   http.StatusBadRequest,
			body:     `{"id":"api.post.get_post_for_thread.root_deleted.app_error"}`,
			wantKind: "gone",
		},
		{
			name:       "500 retryable",
			status:     http.StatusInternalServerError,
			body:       `{"id":"generic"}`,
			wantKind:   "api",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "201 without id is api error",
			status:     http.StatusCreated,
			body:       `{}`,
			wantKind:   "api",
			wantStatus: http.StatusCreated,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			c, stop := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			defer stop()

			// act
			_, err := c.Post(context.Background(), "c", "m", "r")
			require.Error(t, err)

			// assert
			switch tc.wantKind {
			case "gone":
				var got *ThreadGoneError
				require.ErrorAs(t, err, &got)
			case "ratelimited":
				var got *RateLimitedError
				require.ErrorAs(t, err, &got)
				assert.WithinDuration(t, time.Now().Add(42*time.Second), got.Reset, 5*time.Second,
					"RateLimit-Reset seconds become the retry-after instant (SYSREQ-018)")
			case "api":
				var got *APIError
				require.ErrorAs(t, err, &got)
				assert.Equal(t, tc.wantStatus, got.Status)
			}
		})
	}
}

func TestPost_TransportErrorWhenServerDown(t *testing.T) {
	// arrange: closed server, nothing to connect to.
	c, stop := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	stop()

	// act
	_, err := c.Post(context.Background(), "c", "m", "")

	// assert
	var transport *TransportError
	require.ErrorAs(t, err, &transport)
}
