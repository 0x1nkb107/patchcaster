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
	mux.HandleFunc("GET /api/v4/channels/{channel_id}", m.channel)
	mux.HandleFunc("GET /api/v4/channels/{channel_id}/members/{user_id}", m.channelMember)
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

// The bot account as the fake reports it.
const (
	botUserID   = "bot1"
	botUsername = "patchcaster-bot"
)

// The fake's channels besides the suite's own: another one the bot may
// post into, one it was never added to, and an archived one.
const (
	otherChannelID    = "chan-other"
	lobbyChannelID    = "chan-lobby"
	archivedChannelID = "chan-archived"
)

// me answers who the bot token belongs to.
func (m *fakeMattermost) me(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"` + botUserID + `","username":"` + botUsername + `","is_bot":true}`))
}

// channel answers the fake's channels and 404 for any other id, as
// Mattermost does.
func (m *fakeMattermost) channel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("channel_id")
	deleteAt := 0
	switch id {
	case channelID, otherChannelID, lobbyChannelID:
	case archivedChannelID:
		deleteAt = 1700000000000
	default:
		http.Error(w, `{"message":"Unable to find the existing channel."}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"id":%q,"name":%q,"display_name":%q,"delete_at":%d}`, id, id, "Channel "+id, deleteAt)
}

// channelMember answers that the bot is a member of every channel but the
// lobby.
func (m *fakeMattermost) channelMember(w http.ResponseWriter, r *http.Request) {
	id, user := r.PathValue("channel_id"), r.PathValue("user_id")
	member := user == botUserID && (id == channelID || id == otherChannelID || id == archivedChannelID)
	if !member {
		http.Error(w, `{"message":"Unable to find the channel member."}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"channel_id":%q,"user_id":%q}`, id, user)
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
