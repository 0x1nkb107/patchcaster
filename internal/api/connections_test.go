package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// connectionBody is a connection as the tests read it back.
type connectionBody struct {
	ID            int64  `json:"id"`
	Kind          string `json:"kind"`
	BaseURL       string `json:"base_url"`
	Account       string `json:"account"`
	HasCredential bool   `json:"has_credential"`
	WebhookURL    string `json:"webhook_url"`
	Etag          string `json:"etag"`
}

// createConnection posts body as an administrator and returns the answer.
func (f *fixture) createConnection(t *testing.T, body string) *httpResult {
	t.Helper()
	rec := f.serve(t, request{
		method:  http.MethodPost,
		path:    "/api/v1/connections",
		body:    body,
		session: f.sessionOf(t, ada),
	})
	return &httpResult{status: rec.Code, body: rec.Body.Bytes()}
}

// httpResult keeps an answer for decoding.
type httpResult struct {
	status int
	body   []byte
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}

// testSigningToken returns a signing token in GitLab's form. It is built at
// run time, so that secret scanners do not mistake it for a live secret.
func testSigningToken() string {
	return "whsec_" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("g"), 32))
}

func (r *httpResult) connection(t *testing.T) connectionBody {
	t.Helper()
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	var c connectionBody
	require.NoError(t, json.Unmarshal(r.body, &c))
	return c
}

func (r *httpResult) fields(t *testing.T) []string {
	t.Helper()
	var p problemBody
	require.NoError(t, json.Unmarshal(r.body, &p))
	return paths(p)
}

func TestAForgeConnectionGetsAWebhookURL(t *testing.T) {
	// arrange
	f := newFixture(t)

	// act
	created := f.createConnection(t, `{"kind":"gitlab","name":"gitlab","base_url":"https://gitlab.example.org/"}`).connection(t)

	// assert
	assert.Equal(t, "https://gitlab.example.org", created.BaseURL, "the trailing slash goes")
	assert.Equal(t, "https://hooks.example.org/webhooks/1", created.WebhookURL)
	assert.False(t, created.HasCredential)
	assert.Empty(t, created.Account)
	assert.Empty(t, f.platforms.checked, "without a credential there is nothing to check")
}

func TestACredentialIsCheckedAndSealed(t *testing.T) {
	// arrange
	f := newFixture(t)

	// act
	created := f.createConnection(t, `{"kind":"mattermost","name":"chat","base_url":"https://chat.example.org","credential":"bot-token"}`).connection(t)

	// assert
	assert.Equal(t, "bot", created.Account, "the account the platform named")
	assert.True(t, created.HasCredential)
	assert.Empty(t, created.WebhookURL, "a chat connection receives no webhooks")
	assert.Equal(t, []string{"bot-token"}, f.platforms.checked)
	stored := f.store.connections[created.ID]
	opened, err := f.box.Open(stored.Credential, store.ConnectionCredentialBinding(created.ID))
	require.NoError(t, err)
	assert.Equal(t, "bot-token", string(opened))
}

func TestAConnectionIsRefused(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		checkErr   error
		status     int
		wantFields []string
	}{
		{
			"a chat connection without its bot's token",
			`{"kind":"mattermost","name":"chat","base_url":"https://chat.example.org"}`,
			nil, http.StatusUnprocessableEntity, []string{"credential"},
		},
		{
			"a credential the platform refuses",
			`{"kind":"gitlab","name":"gitlab","base_url":"https://gitlab.example.org","credential":"wrong"}`,
			&platform.RefusedError{Platform: platform.GitLab, Answer: "401 Unauthorized"},
			http.StatusUnprocessableEntity, []string{"credential"},
		},
		{
			"an address without the platform",
			`{"kind":"gitlab","name":"gitlab","base_url":"https://example.org","credential":"token"}`,
			&platform.UnreachableError{Platform: platform.GitLab, Err: errors.New("http 404")},
			http.StatusUnprocessableEntity, []string{"base_url"},
		},
		{
			"a base URL that is no URL",
			`{"kind":"gitlab","name":"gitlab","base_url":"gitlab.example.org"}`,
			nil, http.StatusUnprocessableEntity, []string{"base_url"},
		},
		{
			"a kind PatchCaster does not know",
			`{"kind":"jira","name":"jira","base_url":"https://jira.example.org"}`,
			nil, http.StatusBadRequest, []string{"kind"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			f := newFixture(t)
			f.platforms.err = tc.checkErr

			// act
			result := f.createConnection(t, tc.body)

			// assert
			require.Equal(t, tc.status, result.status, string(result.body))
			assert.Equal(t, tc.wantFields, result.fields(t))
			assert.Empty(t, f.store.connections, "a refused connection is not stored")
		})
	}
}

func TestTheRefusalCarriesThePlatformsAnswer(t *testing.T) {
	// arrange
	f := newFixture(t)
	f.platforms.err = &platform.RefusedError{Platform: platform.GitLab, Answer: "401 Unauthorized"}

	// act
	result := f.createConnection(t, `{"kind":"gitlab","name":"gitlab","base_url":"https://gitlab.example.org","credential":"wrong"}`)

	// assert
	var p problemBody
	require.NoError(t, json.Unmarshal(result.body, &p))
	require.Len(t, p.Errors, 1)
	assert.Equal(t, []string{"gitlab refused it: 401 Unauthorized"}, p.Errors[0].Messages)
	assert.NotContains(t, string(result.body), "wrong", "the credential is never echoed")
}

func TestAMovedConnectionIsCheckedWithItsStoredCredential(t *testing.T) {
	// arrange
	f := newFixture(t)
	created := f.createConnection(t, `{"kind":"mattermost","name":"chat","base_url":"https://chat.example.org","credential":"bot-token"}`).connection(t)
	f.platforms.checked = nil
	admin := f.sessionOf(t, ada)

	// act
	renamed := f.serve(t, request{
		method:  http.MethodPut,
		path:    "/api/v1/connections/1",
		body:    `{"name":"team chat","base_url":"https://chat.example.org"}`,
		session: admin,
		header:  map[string]string{"If-Match": created.Etag},
	})
	checkedAfterRename := len(f.platforms.checked)
	moved := f.serve(t, request{
		method:  http.MethodPut,
		path:    "/api/v1/connections/1",
		body:    `{"name":"team chat","base_url":"https://chat.example.net"}`,
		session: admin,
		header:  map[string]string{"If-Match": renamed.Header().Get("ETag")},
	})

	// assert
	require.Equal(t, http.StatusOK, renamed.Code, renamed.Body.String())
	assert.Zero(t, checkedAfterRename, "a new name changes nothing the platform sees")
	require.Equal(t, http.StatusOK, moved.Code, moved.Body.String())
	assert.Equal(t, []string{"bot-token"}, f.platforms.checked, "the new address is checked with the stored token")
}

func TestSigningTokensBelongToForgesInTheirForm(t *testing.T) {
	// arrange
	f := newFixture(t)
	forge := f.createConnection(t, `{"kind":"gitlab","name":"gitlab","base_url":"https://gitlab.example.org"}`).connection(t)
	chat := f.createConnection(t, `{"kind":"mattermost","name":"chat","base_url":"https://chat.example.org","credential":"bot-token"}`).connection(t)
	admin := f.sessionOf(t, ada)
	add := func(connectionID int64, token string) *httpResult {
		rec := f.serve(t, request{
			method:  http.MethodPost,
			path:    "/api/v1/connections/" + itoa(connectionID) + "/signing-tokens",
			body:    `{"label":"group hook","token":"` + token + `"}`,
			session: admin,
		})
		return &httpResult{status: rec.Code, body: rec.Body.Bytes()}
	}

	// act
	added := add(forge.ID, testSigningToken())
	malformed := add(forge.ID, "not-a-token")
	toChat := add(chat.ID, testSigningToken())

	// assert
	require.Equal(t, http.StatusCreated, added.status, string(added.body))
	assert.NotContains(t, string(added.body), "whsec_", "the token is never echoed")
	require.Len(t, f.store.signingTokens, 1)
	for id, stored := range f.store.signingTokens {
		opened, err := f.box.Open(stored.Sealed, store.SigningTokenBinding(id))
		require.NoError(t, err)
		assert.Equal(t, testSigningToken(), string(opened))
	}
	require.Equal(t, http.StatusUnprocessableEntity, malformed.status)
	assert.Equal(t, []string{"token"}, malformed.fields(t))
	assert.Equal(t, http.StatusConflict, toChat.status, "a chat connection receives no webhooks")
}

func TestOnlyAdministratorsManageConnections(t *testing.T) {
	// arrange
	f := newFixture(t)

	// act
	rec := f.serve(t, request{
		method:  http.MethodPost,
		path:    "/api/v1/connections",
		body:    `{"kind":"gitlab","name":"gitlab","base_url":"https://gitlab.example.org"}`,
		session: f.sessionOf(t, bob),
	})

	// assert
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, f.store.connections)
}
