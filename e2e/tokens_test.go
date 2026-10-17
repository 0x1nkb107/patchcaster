package e2e

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// createToken creates an API token named name for the person logged in at b.
func createToken(t *testing.T, b *browser, name string) *apiv1.CreatedApiToken {
	t.Helper()
	created, err := b.api(t).CreateToken(context.Background(), &apiv1.ApiTokenRequest{Name: name})
	require.NoError(t, err)
	return created
}

// logInAs logs who in through a new browser.
func logInAs(t *testing.T, svc *service, provider *fakeProvider, who identity) *browser {
	t.Helper()
	b := newBrowser(t, svc)
	provider.authenticateAs(who, honest)
	b.requireLoggedIn(t, b.logIn(t))
	return b
}

// personNamed finds the person with subject in the first page of people.
func personNamed(t *testing.T, admin *browser, subject string) apiv1.Person {
	t.Helper()
	people, err := admin.api(t).ListPersons(context.Background(), apiv1.ListPersonsParams{})
	require.NoError(t, err)
	for _, person := range people.Items {
		if person.Subject == subject {
			return person
		}
	}
	t.Fatalf("no person %q", subject)
	return apiv1.Person{}
}

// getWith sends a GET to the management listener with one header set, and
// returns the status and body.
func getWith(t *testing.T, svc *service, path, header, value string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, svc.managementURL+path, nil)
	require.NoError(t, err)
	req.Header.Set(header, value)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// A token acts with its owner's permissions, its changes name it in the
// audit log, and its secret is shown only once (SYSREQ-040).
func TestATokenActsAsItsOwner(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	logInAs(t, svc, provider, carol)
	created := createToken(t, admin, "ci")
	ci := automation(t, svc, created.Secret)

	// act
	carolRead := personNamed(t, admin, "carol")
	_, promoteErr := ci.UpdatePerson(ctx, &apiv1.PersonUpdate{Administrator: true},
		apiv1.UpdatePersonParams{ID: carolRead.ID, IfMatch: carolRead.Etag})
	audit, auditErr := admin.api(t).ListAudit(ctx, apiv1.ListAuditParams{})
	_, listing := getWith(t, svc, "/api/v1/tokens", "Cookie", sessionCookie+"="+admin.session(t).Value)
	managedStatus, _ := getWith(t, svc, "/api/v1/tokens", "Authorization", "Bearer "+created.Secret)

	// assert
	require.NoError(t, promoteErr)
	require.NoError(t, auditErr)
	newest := audit.Items[0]
	assert.Equal(t, "person.role", newest.Action)
	assert.Equal(t, "Ada Lovelace", newest.Actor)
	assert.Equal(t, "ci", newest.ActorToken.Or(""))
	assert.Equal(t, created.Token.ID, newest.ActorTokenID.Or(0))
	assert.Contains(t, listing, `"name":"ci"`)
	assert.NotContains(t, listing, created.Secret, "the secret is shown only once")
	assert.Equal(t, http.StatusUnauthorized, managedStatus, "a token cannot manage tokens")
}

// A token never exceeds its owner's current role (SYSREQ-040).
func TestLosingARoleLimitsTheTokenAtOnce(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	member := logInAs(t, svc, provider, carol)
	carolRead := personNamed(t, admin, "carol")
	promoted, err := admin.api(t).UpdatePerson(ctx, &apiv1.PersonUpdate{Administrator: true},
		apiv1.UpdatePersonParams{ID: carolRead.ID, IfMatch: carolRead.Etag})
	require.NoError(t, err)
	ci := automation(t, svc, createToken(t, member, "ci").Secret)
	_, beforeErr := ci.ListPersons(ctx, apiv1.ListPersonsParams{})

	// act
	_, err = admin.api(t).UpdatePerson(ctx, &apiv1.PersonUpdate{Administrator: false},
		apiv1.UpdatePersonParams{ID: carolRead.ID, IfMatch: promoted.ETag.Or("")})
	require.NoError(t, err)
	_, afterErr := ci.ListPersons(ctx, apiv1.ListPersonsParams{})

	// assert
	require.NoError(t, beforeErr)
	requireProblem(t, afterErr, http.StatusForbidden)
}

// A revoked or expired token answers 401 on its next use (SYSREQ-040).
func TestARevokedOrExpiredTokenIsRefused(t *testing.T) {
	// arrange
	ctx := context.Background()
	db := freshDatabase(t)
	svc := startService(t, serviceConfig{mattermostURL: newFakeMattermost(t).url()})
	provider := newFakeProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	revoked := createToken(t, admin, "revoked")
	expired := createToken(t, admin, "expired")
	others := createToken(t, logInAs(t, svc, provider, carol), "carols")

	// act
	revokeErr := admin.api(t).RevokeToken(ctx, apiv1.RevokeTokenParams{ID: revoked.Token.ID})
	_, err := db.Exec("UPDATE api_tokens SET expires_at = now() - interval '1 second' WHERE id = $1", expired.Token.ID)
	require.NoError(t, err)
	_, revokedErr := automation(t, svc, revoked.Secret).GetSession(ctx)
	_, expiredErr := automation(t, svc, expired.Secret).GetSession(ctx)
	othersErr := admin.api(t).RevokeToken(ctx, apiv1.RevokeTokenParams{ID: others.Token.ID})
	_, othersUseErr := automation(t, svc, others.Secret).GetSession(ctx)

	// assert
	require.NoError(t, revokeErr)
	requireProblem(t, revokedErr, http.StatusUnauthorized)
	requireProblem(t, expiredErr, http.StatusUnauthorized)
	requireProblem(t, othersErr, http.StatusNotFound)
	assert.NoError(t, othersUseErr, "nobody revokes another person's token one by one")
}

// An administrator revokes every token and ends every session of a person
// at once, and the person may log in again (SYSREQ-040).
func TestAnAdministratorRevokesAPersonsAccess(t *testing.T) {
	// arrange
	ctx := context.Background()
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	laptop := logInAs(t, svc, provider, bob)
	phone := logInAs(t, svc, provider, bob)
	ci := automation(t, svc, createToken(t, laptop, "ci").Secret)
	bobRead := personNamed(t, admin, "bob")

	// act
	result, revokeErr := admin.api(t).RevokeAccess(ctx, apiv1.RevokeAccessParams{ID: bobRead.ID})
	_, laptopErr := laptop.api(t).GetSession(ctx)
	_, phoneErr := phone.api(t).GetSession(ctx)
	_, tokenErr := ci.GetSession(ctx)
	provider.authenticateAs(bob, honest)
	again := laptop.logIn(t)
	me, againErr := laptop.api(t).GetSession(ctx)

	// assert
	require.NoError(t, revokeErr)
	assert.Equal(t, 1, result.RevokedTokens)
	requireProblem(t, laptopErr, http.StatusUnauthorized)
	requireProblem(t, phoneErr, http.StatusUnauthorized)
	requireProblem(t, tokenErr, http.StatusUnauthorized)
	laptop.requireLoggedIn(t, again)
	require.NoError(t, againErr, "a new login works")
	assert.Equal(t, "bob", me.Subject)
	assert.Contains(t, auditActions(t, admin), "person.access_revoked")
}

// Only administrators revoke other people's access (SYSREQ-039).
func TestAUserCannotRevokeAccess(t *testing.T) {
	// arrange
	svc, provider := startWithProvider(t)
	admin := setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada)
	user := logInAs(t, svc, provider, bob)
	adaRead := personNamed(t, admin, "ada")

	// act
	_, err := user.api(t).RevokeAccess(context.Background(), apiv1.RevokeAccessParams{ID: adaRead.ID})
	_, adaErr := admin.api(t).GetSession(context.Background())

	// assert
	requireProblem(t, err, http.StatusForbidden)
	assert.NoError(t, adaErr)
}
