package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePlatform answers the one account endpoint of kind, for the token
// "good" only.
func fakePlatform(t *testing.T, kind Kind) *url.URL {
	t.Helper()
	mux := http.NewServeMux()
	switch kind {
	case GitLab:
		mux.HandleFunc("GET /gitlab/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("PRIVATE-TOKEN") != "good" {
				http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":7,"username":"patchcaster-bot"}`))
		})
	case Mattermost:
		mux.HandleFunc("GET /api/v4/users/me", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer good" {
				http.Error(w, `{"id":"api.context.session_expired.app_error","message":"Invalid or expired session, please login again."}`, http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":"u1","username":"patchcaster","is_bot":true}`))
		})
		mux.HandleFunc("GET /api/v4/channels/{channel_id}", func(w http.ResponseWriter, r *http.Request) {
			switch r.PathValue("channel_id") {
			case "town", "lobby":
				_, _ = w.Write([]byte(`{"id":"` + r.PathValue("channel_id") + `","name":"town-square","display_name":"Town Square","delete_at":0}`))
			case "old":
				_, _ = w.Write([]byte(`{"id":"old","name":"old","display_name":"Old","delete_at":1700000000000}`))
			case "secret":
				http.Error(w, `{"message":"You do not have the appropriate permissions."}`, http.StatusForbidden)
			default:
				http.Error(w, `{"message":"Unable to find the existing channel."}`, http.StatusNotFound)
			}
		})
		mux.HandleFunc("GET /api/v4/channels/{channel_id}/members/{user_id}", func(w http.ResponseWriter, r *http.Request) {
			if r.PathValue("channel_id") != "town" || r.PathValue("user_id") != "u1" {
				http.Error(w, `{"message":"Unable to find the channel member."}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"channel_id":"town","user_id":"u1"}`))
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	require.NoError(t, err)
	if kind == GitLab {
		base = base.JoinPath("gitlab")
	}
	return base
}

func TestCheckNamesTheAccount(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{GitLab, "patchcaster-bot"},
		{Mattermost, "patchcaster"},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			// act
			account, err := Checker{}.Check(context.Background(), tc.kind, fakePlatform(t, tc.kind), "good")

			// assert
			require.NoError(t, err)
			assert.Equal(t, tc.want, account)
		})
	}
}

func TestCheckPassesOnTheRefusal(t *testing.T) {
	tests := []struct {
		kind   Kind
		answer string
	}{
		{GitLab, "401 Unauthorized"},
		{Mattermost, "Invalid or expired session, please login again."},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			// act
			_, err := Checker{}.Check(context.Background(), tc.kind, fakePlatform(t, tc.kind), "bad")

			// assert
			var refused *RefusedError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, tc.answer, refused.Answer)
		})
	}
}

func TestCheckReportsAnAddressWithoutThePlatform(t *testing.T) {
	// arrange: a server that is no GitLab, and an address nothing listens on.
	elsewhere := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(elsewhere.Close)
	notGitLab, err := url.Parse(elsewhere.URL)
	require.NoError(t, err)
	closed, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)

	for name, base := range map[string]*url.URL{"another server": notGitLab, "no server": closed} {
		t.Run(name, func(t *testing.T) {
			// act
			_, err := Checker{}.Check(context.Background(), GitLab, base, "good")

			// assert
			var unreachable *UnreachableError
			assert.ErrorAs(t, err, &unreachable)
		})
	}
}

func TestParseBaseURL(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"https://gitlab.example.org/", "https://gitlab.example.org", false},
		{"https://example.org/gitlab", "https://example.org/gitlab", false},
		{"gitlab.example.org", "", true},
		{"ftp://gitlab.example.org", "", true},
		{"https://gitlab.example.org/?x=1", "", true},
		{"https://user:pw@gitlab.example.org", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			// act
			u, err := ParseBaseURL(tc.raw)

			// assert
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, u.String())
		})
	}
}

func TestCheckChannelNamesADeliverableChannel(t *testing.T) {
	// act
	name, err := Checker{}.CheckChannel(context.Background(), Mattermost, fakePlatform(t, Mattermost), "good", "town")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "Town Square", name)
}

func TestCheckChannelNamesWhyItCannotDeliver(t *testing.T) {
	tests := []struct {
		channel string
		reason  string
	}{
		{"nowhere", "no channel has this id"},
		{"secret", "the bot may not see the channel: add it to the channel"},
		{"old", "the channel is archived"},
		{"lobby", "the bot is not a member of the channel: add it to the channel"},
	}
	for _, tc := range tests {
		t.Run(tc.channel, func(t *testing.T) {
			// act
			_, err := Checker{}.CheckChannel(context.Background(), Mattermost, fakePlatform(t, Mattermost), "good", tc.channel)

			// assert
			var undeliverable *UndeliverableError
			require.ErrorAs(t, err, &undeliverable)
			assert.Equal(t, tc.reason, undeliverable.Reason)
		})
	}
}

func TestCheckChannelPassesOnARefusedCredential(t *testing.T) {
	// act
	_, err := Checker{}.CheckChannel(context.Background(), Mattermost, fakePlatform(t, Mattermost), "bad", "town")

	// assert
	var refused *RefusedError
	assert.ErrorAs(t, err, &refused)
}
