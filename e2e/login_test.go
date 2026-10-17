package e2e

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcexec "github.com/testcontainers/testcontainers-go/exec"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// The people of the tests, as the fake provider knows them.
var (
	ada   = identity{subject: "ada", name: "Ada Lovelace", groups: []string{"engineering"}}
	bob   = identity{subject: "bob", name: "Bob", groups: []string{"design"}}
	carol = identity{subject: "carol", name: "Carol", groups: []string{"engineering"}}
)

// startWithProvider starts a service on an empty database, and a fake
// identity provider next to it.
func startWithProvider(t *testing.T) (*service, *fakeProvider) {
	t.Helper()
	freshDatabase(t)
	svc := startService(t, serviceConfig{mattermostURL: newFakeMattermost(t).url()})
	return svc, newFakeProvider(t)
}

// requireProblem asserts that an API call failed with a problem of status,
// and returns the problem.
func requireProblem(t *testing.T, err error, status int) apiv1.Problem {
	t.Helper()
	var problem *apiv1.ProblemStatusCode
	require.ErrorAs(t, err, &problem)
	require.Equal(t, status, problem.StatusCode, "detail: %s", problem.Response.Detail.Or(""))
	return problem.Response
}

// auditActions lists the actions in the audit log, newest first.
func auditActions(t *testing.T, admin *browser) []string {
	t.Helper()
	audit, err := admin.api(t).ListAudit(context.Background(), apiv1.ListAuditParams{})
	require.NoError(t, err)
	actions := make([]string, 0, len(audit.Items))
	for _, entry := range audit.Items {
		actions = append(actions, entry.Action)
	}
	return actions
}

// The bootstrap link, the provider settings and one login make the first
// administrator, and then the setup closes (SYSREQ-037, SYSREQ-038).
func TestTheFirstAdministratorSetsUpTheInstance(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	before, err := newBrowser(t, svc).api(t).GetInstance(ctx)
	require.NoError(t, err)
	token := bootstrap(t, svc)

	// act
	admin := setUp(t, svc, provider, setupRequest(token, provider), ada)
	me, sessionErr := admin.api(t).GetSession(ctx)
	after, instanceErr := admin.api(t).GetInstance(ctx)
	_, reuseErr := newBrowser(t, svc).api(t).SetUp(ctx, setupRequest(token, provider))
	out, code := runPatchcaster(t, "bootstrap", "--database-url", databaseURL, "--interface-url", svc.managementURL)

	// assert
	assert.True(t, before.SetupRequired)
	require.NoError(t, sessionErr)
	assert.True(t, me.Administrator)
	assert.Equal(t, "ada", me.Subject)
	assert.Equal(t, "Ada Lovelace", me.DisplayName)
	require.NoError(t, instanceErr)
	assert.False(t, after.SetupRequired)
	requireProblem(t, reuseErr, http.StatusConflict)
	assert.NotZero(t, code)
	assert.Contains(t, out, "an administrator exists")
	assert.Subset(t, auditActions(t, admin), []string{"setup.complete", "login_settings.setup", "setup.link_issued"})
}

// Only the browser that opened the setup link becomes the first
// administrator, even when someone else logs in first (SYSREQ-038).
func TestTheSetupCompletesOnlyInItsOwnBrowser(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	setupBrowser := newBrowser(t, svc)
	started, err := setupBrowser.api(t).SetUp(ctx, setupRequest(bootstrap(t, svc), provider))
	require.NoError(t, err)

	// act
	provider.authenticateAs(bob, honest)
	elsewhere := newBrowser(t, svc).logIn(t)
	provider.authenticateAs(ada, honest)
	inSetup := setupBrowser.visit(t, started.LoginURL)
	me, sessionErr := setupBrowser.api(t).GetSession(ctx)

	// assert
	assert.Equal(t, http.StatusForbidden, elsewhere.status)
	assert.Contains(t, elsewhere.body, "the setup is not complete")
	setupBrowser.requireLoggedIn(t, inSetup)
	require.NoError(t, sessionErr)
	assert.Equal(t, "ada", me.Subject)
	assert.True(t, me.Administrator)
}

func TestAForgedSetupTokenIsRefused(t *testing.T) {
	// arrange
	svc, provider := startWithProvider(t)
	bootstrap(t, svc)

	// act
	_, err := newBrowser(t, svc).api(t).SetUp(context.Background(), setupRequest("forged", provider))

	// assert
	requireProblem(t, err, http.StatusForbidden)
}

// Administrators may limit login to groups. A refused login says why and is
// audited (SYSREQ-037).
func TestLoginIsLimitedToGroups(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	req := setupRequest(bootstrap(t, svc), provider)
	req.GroupsClaim = apiv1.NewOptString("groups")
	req.AllowedGroups = []string{"engineering"}
	admin := setUp(t, svc, provider, req, ada)

	// act
	outsider := newBrowser(t, svc)
	provider.authenticateAs(bob, honest)
	refused := outsider.logIn(t)
	member := newBrowser(t, svc)
	provider.authenticateAs(carol, honest)
	admitted := member.logIn(t)
	me, sessionErr := member.api(t).GetSession(ctx)

	// assert
	assert.Equal(t, http.StatusForbidden, refused.status)
	assert.Contains(t, refused.body, "not a member of a group that may log in")
	assert.Nil(t, outsider.session(t), "a refused login starts no session")
	member.requireLoggedIn(t, admitted)
	require.NoError(t, sessionErr)
	assert.False(t, me.Administrator, "people start with the role User")
	assert.Contains(t, auditActions(t, admin), "login.refused")
}

// An ID token whose signature, audience, issuer or expiry does not check out
// is refused (SYSREQ-037).
func TestAnIDTokenThatDoesNotCheckOutIsRefused(t *testing.T) {
	svc, provider := startWithProvider(t)
	setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)

	tests := []struct {
		name string
		how  tamper
	}{
		{"a foreign signature", foreignSignature},
		{"a foreign audience", foreignAudience},
		{"a foreign issuer", foreignIssuer},
		{"an expired token", expired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			b := newBrowser(t, svc)
			provider.authenticateAs(carol, tc.how)

			// act
			landed := b.logIn(t)

			// assert
			assert.Equal(t, http.StatusUnauthorized, landed.status, landed.body)
			assert.Nil(t, b.session(t))
		})
	}
}

// The recovery command makes a person an administrator, and the grant is
// audited (SYSREQ-038).
func TestAdministratorAccessIsRecovered(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	user := newBrowser(t, svc)
	provider.authenticateAs(bob, honest)
	user.requireLoggedIn(t, user.logIn(t))
	_, beforeErr := user.api(t).ListPersons(ctx, apiv1.ListPersonsParams{})

	// act
	out, code := runPatchcaster(t, "admin", "grant",
		"--database-url", databaseURL,
		"--issuer", provider.issuer(),
		"--subject", "bob")
	me, sessionErr := user.api(t).GetSession(ctx)
	_, afterErr := user.api(t).ListPersons(ctx, apiv1.ListPersonsParams{})

	// assert
	requireProblem(t, beforeErr, http.StatusForbidden)
	assert.Zero(t, code, out)
	require.NoError(t, sessionErr)
	assert.True(t, me.Administrator)
	assert.NoError(t, afterErr)
	assert.Contains(t, auditActions(t, admin), "person.recovery_grant")
}

// Logging out ends the session on the server, not only in the browser
// (SYSREQ-037).
func TestLogoutEndsTheSession(t *testing.T) {
	// arrange
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	kept := admin.session(t)

	// act
	logoutErr := admin.api(t).EndSession(context.Background())
	req, err := http.NewRequest(http.MethodGet, svc.managementURL+"/api/v1/session", nil)
	require.NoError(t, err)
	req.AddCookie(kept)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	// assert
	require.NoError(t, logoutErr)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a copy of the old cookie opens nothing")
}

// Every change sends the version it read, and a stale one is refused
// (ADR-043).
func TestChangesNeedTheCurrentVersion(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	member := newBrowser(t, svc)
	provider.authenticateAs(carol, honest)
	member.requireLoggedIn(t, member.logIn(t))
	people, err := admin.api(t).ListPersons(ctx, apiv1.ListPersonsParams{})
	require.NoError(t, err)
	require.Len(t, people.Items, 2)
	carolRead := people.Items[1]
	settingsRead, err := admin.api(t).GetLoginSettings(ctx)
	require.NoError(t, err)
	limited := &apiv1.LoginSettingsUpdate{
		Issuer:        provider.issuer(),
		ClientID:      oidcClientID,
		GroupsClaim:   "groups",
		AllowedGroups: []string{"engineering"},
	}

	// act
	promoted, promoteErr := admin.api(t).UpdatePerson(ctx, &apiv1.PersonUpdate{Administrator: true},
		apiv1.UpdatePersonParams{ID: carolRead.ID, IfMatch: carolRead.Etag})
	_, stalePersonErr := admin.api(t).UpdatePerson(ctx, &apiv1.PersonUpdate{Administrator: false},
		apiv1.UpdatePersonParams{ID: carolRead.ID, IfMatch: carolRead.Etag})
	changed, changeErr := admin.api(t).PutLoginSettings(ctx, limited,
		apiv1.PutLoginSettingsParams{IfMatch: settingsRead.ETag.Or("")})
	_, staleSettingsErr := admin.api(t).PutLoginSettings(ctx, limited,
		apiv1.PutLoginSettingsParams{IfMatch: settingsRead.ETag.Or("")})

	// assert
	require.NoError(t, promoteErr)
	assert.True(t, promoted.Response.Administrator)
	assert.NotEqual(t, carolRead.Etag, promoted.ETag.Or(""))
	requireProblem(t, stalePersonErr, http.StatusPreconditionFailed)
	require.NoError(t, changeErr)
	assert.Equal(t, []string{"engineering"}, changed.Response.AllowedGroups)
	requireProblem(t, staleSettingsErr, http.StatusPreconditionFailed)
	assert.Subset(t, auditActions(t, admin), []string{"person.role", "login_settings.update"})
}

func TestAnIssuerWithoutAProviderIsRefused(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	settingsRead, err := admin.api(t).GetLoginSettings(ctx)
	require.NoError(t, err)

	// act
	_, putErr := admin.api(t).PutLoginSettings(ctx, &apiv1.LoginSettingsUpdate{
		Issuer:        "http://" + freeAddress(t),
		ClientID:      oidcClientID,
		AllowedGroups: []string{},
	}, apiv1.PutLoginSettingsParams{IfMatch: settingsRead.ETag.Or("")})

	// assert
	problem := requireProblem(t, putErr, http.StatusUnprocessableEntity)
	require.Len(t, problem.Errors, 1)
	assert.Equal(t, "issuer", problem.Errors[0].Path)
}

// A browser's cross-site request that would change something is refused
// (SYSREQ-037).
func TestCrossSiteChangesAreRefused(t *testing.T) {
	// arrange
	freshDatabase(t)
	svc := startService(t, serviceConfig{mattermostURL: newFakeMattermost(t).url()})

	tests := []struct {
		name   string
		header map[string]string
		status int
	}{
		{"a cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"a foreign origin", map[string]string{"Origin": "https://elsewhere.example"}, http.StatusForbidden},
		{"the interface's own origin", map[string]string{"Origin": svc.managementURL}, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			req, err := http.NewRequest(http.MethodPost, svc.managementURL+"/api/v1/setup", strings.NewReader("{}"))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			for name, value := range tc.header {
				req.Header.Set(name, value)
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			// assert
			assert.Equal(t, tc.status, resp.StatusCode)
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
		})
	}
}

// dumpDatabase returns what a backup of the suite database holds, as
// pg_dump writes it.
func dumpDatabase(t *testing.T) string {
	t.Helper()
	code, out, err := databaseContainer.Exec(context.Background(),
		[]string{"pg_dump", "--username", "patchcaster", "patchcaster"}, tcexec.Multiplexed())
	require.NoError(t, err)
	dump, err := io.ReadAll(out)
	require.NoError(t, err)
	require.Zero(t, code, string(dump))
	return string(dump)
}

// Neither a backup nor the logs reveal the client secret or the setup token
// (SYSREQ-042).
func TestSecretsStaySealedAtRest(t *testing.T) {
	// arrange
	svc, provider := startWithProvider(t)
	token := bootstrap(t, svc)
	setUp(t, svc, provider, setupRequest(token, provider), ada)

	// act
	dump := dumpDatabase(t)
	logs := svc.logs.String()

	// assert
	require.Contains(t, dump, "login_settings", "the dump holds the settings")
	for _, secret := range []string{oidcClientSecret, token} {
		assert.NotContains(t, dump, secret)
		assert.NotContains(t, dump, hex.EncodeToString([]byte(secret)), "nor as bytea")
		assert.NotContains(t, logs, secret)
	}
}

// After a key rotation and a reseal, the new key alone still opens the
// client secret, so people can still log in (ADR-035).
func TestTheClientSecretSurvivesAKeyRotation(t *testing.T) {
	// arrange
	freshDatabase(t)
	mattermost := newFakeMattermost(t)
	provider := newFakeProvider(t)
	first := startService(t, serviceConfig{mattermostURL: mattermost.url()})
	setUp(t, first, provider, setupRequest(bootstrap(t, first), provider), ada)
	first.stop(t)

	// act
	resealOut, resealCode := runPatchcaster(t, "secrets", "reseal",
		"--database-url", databaseURL,
		"--encryption-key", rotatedKey,
		"--previous-encryption-key", suiteKey)
	after := startService(t, serviceConfig{mattermostURL: mattermost.url(), encryptionKey: rotatedKey})
	b := newBrowser(t, after)
	provider.authenticateAs(carol, honest)
	landed := b.logIn(t)

	// assert
	assert.Zero(t, resealCode, resealOut)
	assert.Contains(t, resealOut, "count=2", "the key check and the client secret")
	b.requireLoggedIn(t, landed)
}
