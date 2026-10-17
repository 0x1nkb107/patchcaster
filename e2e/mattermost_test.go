package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// botToken is the Mattermost bot token the service is started with.
const botToken = "e2e-bot-token"

// post is a message that the fake Mattermost accepted.
type post struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	RootID    string `json:"root_id"`
}

// fakeMattermost answers the two calls the bot client makes: creating a post
// and updating one. It records what it accepted. It can answer every new
// post with 503 to simulate an outage, and it can hold new posts in flight
// until the test releases them.
//
// Every request is first checked against the published Mattermost API
// (ADR-027). A request the real server would reject is answered with 400,
// as Mattermost answers it, and fails the test when the test ends.
type fakeMattermost struct {
	server *httptest.Server
	held   chan struct{} // receives one value for every post that starts waiting

	mu        sync.Mutex
	accepted  []post        // posts answered with 201, in arrival order
	updated   []string      // ids of the posts updated with PUT, in arrival order
	attempts  int           // create requests received, whatever the answer
	abandoned int           // held posts whose client went away before the release
	failing   bool          // answer create requests with 503 while set
	hold      chan struct{} // while non-nil, new posts wait until it is closed
	breaches  []string      // requests that broke the Mattermost API, as messages
}

func newFakeMattermost(t *testing.T) *fakeMattermost {
	t.Helper()
	api, err := loadMattermostAPI()
	require.NoError(t, err)

	m := &fakeMattermost{held: make(chan struct{}, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/posts", m.createPost)
	mux.HandleFunc("PUT /api/v4/posts/{id}", m.updatePost)
	mux.HandleFunc("GET /api/v4/users/me", m.me)
	m.server = httptest.NewServer(m.enforceAPI(api, requireBotToken(mux)))
	t.Cleanup(func() {
		// Close waits for every request, so held posts must go first.
		m.release()
		m.server.Close()
		if breaches := m.apiBreaches(); len(breaches) > 0 {
			t.Errorf("the service called Mattermost outside its API:\n%s", strings.Join(breaches, "\n"))
		}
	})
	return m
}

// enforceAPI answers a request that breaks the Mattermost API with 400 and
// records it, and passes every valid call on.
func (m *fakeMattermost) enforceAPI(api *mattermostAPI, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := api.check(r.Context(), r); err != nil {
			m.mu.Lock()
			m.breaches = append(m.breaches, err.Error())
			m.mu.Unlock()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireBotToken answers 401 to any request without the bot token, as
// Mattermost does.
func requireBotToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+botToken {
			http.Error(w, "missing or wrong bot token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *fakeMattermost) url() string { return m.server.URL }

// botUsername is the bot account's name, as the fake reports it.
const botUsername = "patchcaster-bot"

// me answers who the bot token belongs to.
func (m *fakeMattermost) me(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"bot1","username":"` + botUsername + `","is_bot":true}`))
}

func (m *fakeMattermost) createPost(w http.ResponseWriter, r *http.Request) {
	var p post
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "decode post", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.attempts++
	failing, hold := m.failing, m.hold
	m.mu.Unlock()

	if failing {
		http.Error(w, "mattermost is down", http.StatusServiceUnavailable)
		return
	}
	if hold != nil && !m.waitForRelease(r.Context(), hold) {
		return
	}
	m.accept(w, p)
}

// waitForRelease holds a post until the test releases it. It reports false
// when the client goes away first, which is what a killed service does.
func (m *fakeMattermost) waitForRelease(ctx context.Context, hold chan struct{}) bool {
	m.held <- struct{}{}
	select {
	case <-hold:
		return true
	case <-ctx.Done():
		m.mu.Lock()
		m.abandoned++
		m.mu.Unlock()
		return false
	}
}

func (m *fakeMattermost) accept(w http.ResponseWriter, p post) {
	m.mu.Lock()
	p.ID = fmt.Sprintf("post-%d", len(m.accepted)+1)
	m.accepted = append(m.accepted, p)
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": p.ID})
}

func (m *fakeMattermost) updatePost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m.mu.Lock()
	m.updated = append(m.updated, id)
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// setFailing switches the simulated outage on or off.
func (m *fakeMattermost) setFailing(failing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failing = failing
}

// holdPosts makes new posts wait until release is called.
func (m *fakeMattermost) holdPosts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hold = make(chan struct{})
}

// release lets held posts through and stops holding new ones. It is safe to
// call when nothing is held.
func (m *fakeMattermost) release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hold != nil {
		close(m.hold)
		m.hold = nil
	}
}

// awaitHeld waits until a post is held in flight.
func (m *fakeMattermost) awaitHeld(t *testing.T) {
	t.Helper()
	select {
	case <-m.held:
	case <-time.After(10 * time.Second):
		t.Fatal("no post reached the fake Mattermost")
	}
}

func (m *fakeMattermost) acceptedPosts() []post {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]post(nil), m.accepted...)
}

func (m *fakeMattermost) updatedPosts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.updated...)
}

func (m *fakeMattermost) attemptCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attempts
}

func (m *fakeMattermost) apiBreaches() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.breaches...)
}

func (m *fakeMattermost) abandonedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.abandoned
}
