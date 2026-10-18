package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

var errNotInTest = errors.New("not used by this test")

// fakeStore keeps what the tests set up in memory and records what the
// operations asked for.
type fakeStore struct {
	admins         int
	persons        map[int64]*store.Person
	people         []store.Person
	peopleAfter    int64
	setupLinkValid bool
	setupSettings  *store.LoginSettings
	tokens         map[string]*store.APIToken // by hash
	connections    map[int64]*store.Connection
	signingTokens  map[int64]store.NewSigningToken
	nextID         int64

	// The health the store reports, by id. Missing ids have none yet.
	forgeHealth map[int64]store.ForgeHealth
	chatHealth  map[int64]store.SinkHealth
}

func (f *fakeStore) AdministratorCount(context.Context) (int, error) {
	return f.admins, nil
}

func (f *fakeStore) Person(_ context.Context, id int64) (*store.Person, error) {
	person, ok := f.persons[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return person, nil
}

func (f *fakeStore) People(_ context.Context, afterID int64, limit int) ([]store.Person, error) {
	f.peopleAfter = afterID
	var page []store.Person
	for _, p := range f.people {
		if p.ID > afterID && len(page) < limit {
			page = append(page, p)
		}
	}
	return page, nil
}

func (f *fakeStore) SetAdministrator(context.Context, store.Actor, int64, bool, int64, time.Time) (*store.Person, error) {
	return nil, errNotInTest
}

func (f *fakeStore) LoginSettings(context.Context) (*store.LoginSettings, error) {
	return nil, errNotInTest
}

func (f *fakeStore) UpdateLoginSettings(context.Context, store.Actor, store.LoginSettings, time.Time) (*store.LoginSettings, error) {
	return nil, errNotInTest
}

func (f *fakeStore) SetupLinkValid(context.Context, []byte, time.Time) (bool, error) {
	return f.setupLinkValid, nil
}

func (f *fakeStore) PutSetupLoginSettings(_ context.Context, _ store.Actor, _ []byte, s store.LoginSettings, _ time.Time) error {
	f.setupSettings = &s
	return nil
}

func (f *fakeStore) AuditEntries(context.Context, int64, int) ([]store.AuditEntry, error) {
	return nil, errNotInTest
}

func (f *fakeStore) CreateToken(_ context.Context, _ store.Actor, t store.NewAPIToken, now time.Time) (*store.APIToken, error) {
	token := &store.APIToken{
		ID:        int64(len(f.tokens) + 1),
		PersonID:  t.PersonID,
		Name:      t.Name,
		CreatedAt: now,
		ExpiresAt: t.ExpiresAt,
	}
	f.tokens[string(t.Hash)] = token
	return token, nil
}

func (f *fakeStore) AuthenticateToken(_ context.Context, hash []byte, now time.Time) (*store.APIToken, error) {
	token, ok := f.tokens[string(hash)]
	if !ok || !now.Before(token.ExpiresAt) {
		return nil, store.ErrNotFound
	}
	return token, nil
}

func (f *fakeStore) Tokens(context.Context, int64, int64, int) ([]store.APIToken, error) {
	return nil, errNotInTest
}

func (f *fakeStore) RevokeToken(context.Context, store.Actor, int64, int64, time.Time) error {
	return errNotInTest
}

func (f *fakeStore) RevokeAccess(context.Context, store.Actor, int64, time.Time) (int, error) {
	return 0, errNotInTest
}

func (f *fakeStore) reserve() int64 {
	f.nextID++
	return f.nextID
}

func (f *fakeStore) ReserveConnectionID(context.Context) (int64, error) {
	return f.reserve(), nil
}

func (f *fakeStore) CreateConnection(_ context.Context, _ store.Actor, c store.Connection, _ time.Time) (*store.Connection, error) {
	c.Version = 1
	f.connections[c.ID] = &c
	return &c, nil
}

func (f *fakeStore) Connection(_ context.Context, id int64) (*store.Connection, error) {
	c, ok := f.connections[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return c, nil
}

func (f *fakeStore) Connections(context.Context, int64, int) ([]store.Connection, error) {
	return nil, errNotInTest
}

func (f *fakeStore) UpdateConnection(_ context.Context, _ store.Actor, c store.Connection, _ time.Time) (*store.Connection, error) {
	current := f.connections[c.ID]
	if c.Credential == nil {
		c.Credential = current.Credential
	}
	c.Kind = current.Kind
	c.Version = current.Version + 1
	f.connections[c.ID] = &c
	return &c, nil
}

func (f *fakeStore) DeleteConnection(context.Context, store.Actor, int64, int64, time.Time) error {
	return errNotInTest
}

func (f *fakeStore) ReserveSigningTokenID(context.Context) (int64, error) {
	return f.reserve(), nil
}

func (f *fakeStore) AddSigningToken(_ context.Context, _ store.Actor, t store.NewSigningToken, now time.Time) (*store.SigningToken, error) {
	f.signingTokens[t.ID] = t
	return &store.SigningToken{ID: t.ID, ConnectionID: t.ConnectionID, Label: t.Label, CreatedAt: now}, nil
}

func (f *fakeStore) SigningTokens(context.Context, int64, int64, int) ([]store.SigningToken, error) {
	return nil, errNotInTest
}

func (f *fakeStore) RemoveSigningToken(context.Context, store.Actor, int64, int64, time.Time) error {
	return errNotInTest
}

// The routing configuration is tested end to end against PostgreSQL, so
// the fake has none of it.

func (f *fakeStore) CreateTeam(context.Context, store.Actor, string, time.Time) (*store.Team, error) {
	return nil, errNotInTest
}

func (f *fakeStore) Team(context.Context, int64) (*store.Team, error) { return nil, errNotInTest }

func (f *fakeStore) Teams(context.Context, int64, int64, int) ([]store.Team, error) {
	return nil, errNotInTest
}

func (f *fakeStore) UpdateTeam(context.Context, store.Actor, store.Team, time.Time) (*store.Team, error) {
	return nil, errNotInTest
}

func (f *fakeStore) DeleteTeam(context.Context, store.Actor, int64, int64, time.Time) error {
	return errNotInTest
}

func (f *fakeStore) OperatesTeam(context.Context, int64, int64) (bool, error) {
	return false, errNotInTest
}

func (f *fakeStore) CreateSource(context.Context, store.Actor, store.Source, time.Time) (*store.Source, error) {
	return nil, errNotInTest
}

func (f *fakeStore) Source(context.Context, int64) (*store.Source, error) { return nil, errNotInTest }

func (f *fakeStore) Sources(context.Context, int64, int64, int) ([]store.Source, error) {
	return nil, errNotInTest
}

func (f *fakeStore) UpdateSource(context.Context, store.Actor, store.Source, time.Time) (*store.Source, error) {
	return nil, errNotInTest
}

func (f *fakeStore) DeleteSource(context.Context, store.Actor, int64, int64, time.Time) error {
	return errNotInTest
}

func (f *fakeStore) CreateSink(context.Context, store.Actor, store.Sink, time.Time) (*store.Sink, error) {
	return nil, errNotInTest
}

func (f *fakeStore) Sink(context.Context, int64) (*store.Sink, error) { return nil, errNotInTest }

func (f *fakeStore) SinkHealth(context.Context, []int64) (map[int64]store.SinkHealth, error) {
	return map[int64]store.SinkHealth{}, nil
}

func (f *fakeStore) ForgeHealth(_ context.Context, ids []int64) (map[int64]store.ForgeHealth, error) {
	health := map[int64]store.ForgeHealth{}
	for _, id := range ids {
		health[id] = f.forgeHealth[id]
	}
	return health, nil
}

func (f *fakeStore) ChatConnectionHealth(_ context.Context, ids []int64) (map[int64]store.SinkHealth, error) {
	health := map[int64]store.SinkHealth{}
	for _, id := range ids {
		health[id] = f.chatHealth[id]
	}
	return health, nil
}

func (f *fakeStore) Sinks(context.Context, int64, int64, int) ([]store.Sink, error) {
	return nil, errNotInTest
}

func (f *fakeStore) UpdateSink(context.Context, store.Actor, store.Sink, time.Time) (*store.Sink, error) {
	return nil, errNotInTest
}

func (f *fakeStore) DeleteSink(context.Context, store.Actor, int64, int64, time.Time) error {
	return errNotInTest
}

func (f *fakeStore) CreateRoute(context.Context, store.Actor, store.Route, time.Time) (*store.Route, error) {
	return nil, errNotInTest
}

func (f *fakeStore) Route(context.Context, int64) (*store.Route, error) { return nil, errNotInTest }

func (f *fakeStore) Routes(context.Context, int64, int64, int64, int) ([]store.Route, error) {
	return nil, errNotInTest
}

func (f *fakeStore) UpdateRoute(context.Context, store.Actor, store.Route, time.Time) (*store.Route, error) {
	return nil, errNotInTest
}

func (f *fakeStore) DeleteRoute(context.Context, store.Actor, int64, int64, time.Time) error {
	return errNotInTest
}

// fakePlatforms answers every check with err, or with the account "bot".
type fakePlatforms struct {
	err     error
	checked []string // the credentials it was asked about
}

func (f *fakePlatforms) CheckChannel(context.Context, platform.Kind, *url.URL, string, string) (string, error) {
	return "", errNotInTest
}

func (f *fakePlatforms) Check(_ context.Context, _ platform.Kind, _ *url.URL, credential string) (string, error) {
	f.checked = append(f.checked, credential)
	if f.err != nil {
		return "", f.err
	}
	return "bot", nil
}

// fakeLogin answers the provider check with checkErr.
type fakeLogin struct{ checkErr error }

func (f *fakeLogin) Check(context.Context, string) error { return f.checkErr }

func (f *fakeLogin) URL() string { return "http://interface.example/auth/login" }

// fixture is the API over fakes, behind the session middleware as served.
type fixture struct {
	store     *fakeStore
	login     *fakeLogin
	platforms *fakePlatforms
	box       *seal.Box
	sessions  *scs.SessionManager
	svc       *Service
	handler   http.Handler
}

// The people of the tests. Each fixture keeps copies, which tests may
// change.
var (
	ada = store.Person{ID: 1, Issuer: "https://idp.example", Subject: "ada", Administrator: true, Version: 3, SessionEpoch: 1}
	bob = store.Person{ID: 2, Issuer: "https://idp.example", Subject: "bob", Version: 1, SessionEpoch: 1}
)

func newFixture(t *testing.T) *fixture {
	t.Helper()
	key, err := seal.ParseKey(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	require.NoError(t, err)
	f := &fixture{
		store: &fakeStore{
			admins:        1,
			persons:       map[int64]*store.Person{ada.ID: new(ada), bob.ID: new(bob)},
			tokens:        map[string]*store.APIToken{},
			connections:   map[int64]*store.Connection{},
			signingTokens: map[int64]store.NewSigningToken{},
		},
		login:     &fakeLogin{},
		platforms: &fakePlatforms{},
		box:       seal.NewBox(key),
		sessions:  auth.NewSessions(memstore.New(), false),
	}
	f.svc = NewService(Config{
		Version:   "1.2.3",
		Store:     f.store,
		Box:       f.box,
		Sessions:  f.sessions,
		Login:     f.login,
		Platforms: f.platforms,
		PublicURL: &url.URL{Scheme: "https", Host: "hooks.example.org"},
		Log:       slog.New(slog.DiscardHandler),
	})
	mux := http.NewServeMux()
	require.NoError(t, Routes(mux, f.svc))
	f.handler = f.sessions.LoadAndSave(mux)
	return f
}

// sessionOf logs person in and returns their session cookie.
func (f *fixture) sessionOf(t *testing.T, person store.Person) *http.Cookie {
	t.Helper()
	start := f.sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, auth.StartSession(r.Context(), f.sessions, f.store.persons[person.ID]))
	}))
	rec := httptest.NewRecorder()
	start.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

// request describes one API request. A body is sent as JSON.
type request struct {
	method  string
	path    string
	body    string
	session *http.Cookie
	header  map[string]string
}

func (f *fixture) serve(t *testing.T, r request) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
	if r.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.session != nil {
		req.AddCookie(r.session)
	}
	for name, value := range r.header {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

// problemBody is the RFC 9457 shape the tests read back.
type problemBody struct {
	Type   string       `json:"type"`
	Title  string       `json:"title"`
	Status int          `json:"status"`
	Detail string       `json:"detail"`
	Errors []fieldError `json:"errors"`
}

type fieldError struct {
	Path     string   `json:"path"`
	Messages []string `json:"messages"`
}

func readProblem(t *testing.T, rec *httptest.ResponseRecorder) problemBody {
	t.Helper()
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var p problemBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	return p
}

// paths lists the paths of a problem's field errors.
func paths(p problemBody) []string {
	out := make([]string, 0, len(p.Errors))
	for _, field := range p.Errors {
		out = append(out, field.Path)
	}
	return out
}

func TestGetInstanceReportsTheVersionAndTheSetup(t *testing.T) {
	tests := []struct {
		name   string
		admins int
		want   string
	}{
		{"before the setup", 0, `{"version":"1.2.3","setup_required":true}`},
		{"after the setup", 1, `{"version":"1.2.3","setup_required":false}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			f := newFixture(t)
			f.store.admins = tc.admins

			// act
			rec := f.serve(t, request{method: http.MethodGet, path: "/api/v1/instance"})

			// assert
			require.Equal(t, http.StatusOK, rec.Code)
			assert.JSONEq(t, tc.want, rec.Body.String())
		})
	}
}

func TestTheDescriptionIsServed(t *testing.T) {
	// act
	rec := newFixture(t).serve(t, request{method: http.MethodGet, path: "/api/v1/openapi.yaml"})

	// assert
	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, descriptionV1, body)
}

func TestErrorsAreProblems(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name   string
		req    request
		status int
	}{
		{"an unknown API path", request{method: http.MethodGet, path: "/api/v1/nothing-here"}, http.StatusNotFound},
		{"an unknown API version", request{method: http.MethodGet, path: "/api/v0/instance"}, http.StatusNotFound},
		{"a method the operation does not take", request{method: http.MethodDelete, path: "/api/v1/instance"}, http.StatusMethodNotAllowed},
		{"no session", request{method: http.MethodGet, path: "/api/v1/session"}, http.StatusUnauthorized},
		{"a session that does not exist", request{
			method:  http.MethodGet,
			path:    "/api/v1/session",
			session: &http.Cookie{Name: auth.SessionCookie, Value: "forged"},
		}, http.StatusUnauthorized},
		{"a user managing people", request{
			method:  http.MethodGet,
			path:    "/api/v1/persons",
			session: f.sessionOf(t, bob),
		}, http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			rec := f.serve(t, tc.req)

			// assert
			require.Equal(t, tc.status, rec.Code)
			p := readProblem(t, rec)
			assert.Equal(t, problemType, p.Type)
			assert.Equal(t, tc.status, p.Status)
			assert.Equal(t, http.StatusText(tc.status), p.Title)
		})
	}
}

func TestAMethodNotAllowedNamesTheAllowedOnes(t *testing.T) {
	// act
	rec := newFixture(t).serve(t, request{method: http.MethodPost, path: "/api/v1/instance"})

	// assert
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET", rec.Header().Get("Allow"))
}

// Validation errors name each field the way form libraries do (ADR-043).
func TestInvalidRequestsNameTheirFields(t *testing.T) {
	f := newFixture(t)
	admin := f.sessionOf(t, ada)
	tests := []struct {
		name       string
		req        request
		status     int
		wantFields []string
	}{
		{"a missing field", request{
			method: http.MethodPost,
			path:   "/api/v1/setup",
			body:   `{"setup_token":"t","issuer":"https://idp.example","client_id":"c"}`,
		}, http.StatusBadRequest, []string{"client_secret"}},
		{"two empty fields", request{
			method: http.MethodPost,
			path:   "/api/v1/setup",
			body:   `{"setup_token":"","issuer":"","client_id":"c","client_secret":"s"}`,
		}, http.StatusBadRequest, []string{"setup_token", "issuer"}},
		{"a value of the wrong type", request{
			method: http.MethodPost,
			path:   "/api/v1/setup",
			body:   `{"setup_token":"t","issuer":"i","client_id":"c","client_secret":"s","allowed_groups":[1]}`,
		}, http.StatusBadRequest, []string{}},
		{"a body that is not JSON", request{
			method: http.MethodPost,
			path:   "/api/v1/setup",
			body:   `{"setup_token":`,
		}, http.StatusBadRequest, []string{}},
		{"a body that is not declared JSON", request{
			method: http.MethodPost,
			path:   "/api/v1/setup",
			body:   `{}`,
			header: map[string]string{"Content-Type": "text/plain"},
		}, http.StatusUnsupportedMediaType, []string{}},
		{"a limit out of range", request{
			method:  http.MethodGet,
			path:    "/api/v1/persons?limit=0",
			session: admin,
		}, http.StatusBadRequest, []string{"limit"}},
		{"a cursor this API did not issue", request{
			method:  http.MethodGet,
			path:    "/api/v1/persons?cursor=forged",
			session: admin,
		}, http.StatusBadRequest, []string{"cursor"}},
		{"a change without If-Match", request{
			method:  http.MethodPatch,
			path:    "/api/v1/persons/2",
			body:    `{"administrator":true}`,
			session: admin,
		}, http.StatusPreconditionRequired, []string{}},
		{"a change with an If-Match of no version", request{
			method:  http.MethodPatch,
			path:    "/api/v1/persons/2",
			body:    `{"administrator":true}`,
			session: admin,
			header:  map[string]string{"If-Match": `"latest"`},
		}, http.StatusPreconditionFailed, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			rec := f.serve(t, tc.req)

			// assert
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			p := readProblem(t, rec)
			assert.Equal(t, tc.wantFields, paths(p))
			assert.NotEmpty(t, p.Detail)
		})
	}
}

func TestARequestProblemNeverEchoesTheBody(t *testing.T) {
	// act
	rec := newFixture(t).serve(t, request{
		method: http.MethodPost,
		path:   "/api/v1/setup",
		body:   `{"setup_token":"t","issuer":"i","client_id":"c","client_secret":"hunter2","allowed_groups":[1]}`,
	})

	// assert
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.NotContains(t, rec.Body.String(), "hunter2")
}

func TestPagesCarryTheNextCursor(t *testing.T) {
	// arrange
	f := newFixture(t)
	f.store.people = []store.Person{ada, bob, {ID: 3, Subject: "carol", Version: 1}}
	admin := f.sessionOf(t, ada)

	// act
	first := f.serve(t, request{method: http.MethodGet, path: "/api/v1/persons?limit=2", session: admin})
	var page struct {
		Items      []struct{ Subject string } `json:"items"`
		NextCursor string                     `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	second := f.serve(t, request{method: http.MethodGet, path: "/api/v1/persons?limit=2&cursor=" + page.NextCursor, session: admin})

	// assert
	require.Equal(t, http.StatusOK, first.Code)
	require.Len(t, page.Items, 2)
	assert.NotEmpty(t, page.NextCursor)
	require.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, bob.ID, f.store.peopleAfter, "the next page continues after the last item")
	assert.JSONEq(t, `{"items":[{"id":3,"issuer":"","subject":"carol","display_name":"","email":"","administrator":false,"etag":"\"1\""}]}`, second.Body.String())
}

func TestSetupIsRefused(t *testing.T) {
	body := `{"setup_token":"t","issuer":"https://idp.example","client_id":"c","client_secret":"s"}`
	tests := []struct {
		name       string
		admins     int
		linkValid  bool
		checkErr   error
		status     int
		wantFields []string
	}{
		{"once an administrator exists", 1, true, nil, http.StatusConflict, []string{}},
		{"without a valid setup link", 0, false, nil, http.StatusForbidden, []string{}},
		{"when no provider answers", 0, true, errors.New("discover: connection refused"), http.StatusUnprocessableEntity, []string{"issuer"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			f := newFixture(t)
			f.store.admins = tc.admins
			f.store.setupLinkValid = tc.linkValid
			f.login.checkErr = tc.checkErr

			// act
			rec := f.serve(t, request{method: http.MethodPost, path: "/api/v1/setup", body: body})

			// assert
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.wantFields, paths(readProblem(t, rec)))
			assert.Nil(t, f.store.setupSettings, "a refused setup stores nothing")
		})
	}
}

func TestSetupSealsTheSecretAndStartsTheLogin(t *testing.T) {
	// arrange
	f := newFixture(t)
	f.store.admins = 0
	f.store.setupLinkValid = true

	// act
	rec := f.serve(t, request{
		method: http.MethodPost,
		path:   "/api/v1/setup",
		body:   `{"setup_token":"t","issuer":"https://idp.example","client_id":"c","client_secret":"hunter2","allowed_groups":["ops"]}`,
	})

	// assert
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"login_url":"http://interface.example/auth/login"}`, rec.Body.String())
	require.NotNil(t, f.store.setupSettings)
	assert.Equal(t, []string{"ops"}, f.store.setupSettings.AllowedGroups)
	assert.NotContains(t, string(f.store.setupSettings.ClientSecret), "hunter2")
	secret, err := f.box.Open(f.store.setupSettings.ClientSecret, auth.ClientSecretBinding)
	require.NoError(t, err)
	assert.Equal(t, "hunter2", string(secret))
	assert.NotEmpty(t, rec.Result().Cookies(), "the session remembers the setup link")
}

func TestAnUnexpectedErrorRevealsNothing(t *testing.T) {
	// arrange
	svc := newFixture(t).svc

	// act
	problem := svc.NewError(context.Background(), errors.New("password=hunter2 in a driver error"))

	// assert
	assert.Equal(t, http.StatusInternalServerError, problem.StatusCode)
	assert.False(t, problem.Response.Detail.IsSet(), "an internal error must not echo its cause")
}

func TestAProblemReturnedByAHandlerKeepsItsStatus(t *testing.T) {
	// arrange
	svc := newFixture(t).svc
	conflict := newProblem(http.StatusConflict, "the resource changed meanwhile")

	// act
	problem := svc.NewError(context.Background(), conflict)

	// assert
	assert.Same(t, conflict, problem)
}

func TestATokenActsAsItsOwner(t *testing.T) {
	// arrange
	f := newFixture(t)
	created := f.serve(t, request{
		method:  http.MethodPost,
		path:    "/api/v1/tokens",
		body:    `{"name":"ci"}`,
		session: f.sessionOf(t, ada),
	})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var token struct {
		Token struct {
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"token"`
		Secret string `json:"secret"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &token))
	bearer := map[string]string{"Authorization": "Bearer " + token.Secret}

	// act
	listed := f.serve(t, request{method: http.MethodGet, path: "/api/v1/persons", header: bearer})
	minted := f.serve(t, request{method: http.MethodPost, path: "/api/v1/tokens", body: `{"name":"next"}`, header: bearer})

	// assert
	assert.True(t, strings.HasPrefix(token.Secret, auth.TokenPrefix))
	assert.WithinDuration(t, time.Now().Add(90*24*time.Hour), token.Token.ExpiresAt, time.Minute, "a token lives 90 days unless told otherwise")
	assert.Equal(t, http.StatusOK, listed.Code, "the token acts as its administrator owner")
	assert.Equal(t, http.StatusUnauthorized, minted.Code, "a token cannot create tokens")
}

func TestUnknownCredentialsAreUnauthorized(t *testing.T) {
	// arrange
	f := newFixture(t)
	stale := f.sessionOf(t, ada)
	f.store.persons[ada.ID].SessionEpoch++

	tests := []struct {
		name string
		req  request
	}{
		{"an unknown token", request{
			method: http.MethodGet,
			path:   "/api/v1/session",
			header: map[string]string{"Authorization": "Bearer " + auth.TokenPrefix + "forged"},
		}},
		{"a session from before the access was revoked", request{
			method:  http.MethodGet,
			path:    "/api/v1/session",
			session: stale,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			rec := f.serve(t, tc.req)

			// assert
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestATokenLivesAtMost90Days(t *testing.T) {
	// arrange
	f := newFixture(t)

	// act
	rec := f.serve(t, request{
		method:  http.MethodPost,
		path:    "/api/v1/tokens",
		body:    `{"name":"forever","lifetime_days":91}`,
		session: f.sessionOf(t, ada),
	})

	// assert
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, []string{"lifetime_days"}, paths(readProblem(t, rec)))
}
