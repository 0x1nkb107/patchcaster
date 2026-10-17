package e2e

import (
	"context"
	"encoding/hex"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// platforms is a service with an administrator logged in through the fake
// identity provider, next to a fake GitLab API and the fake Mattermost it
// posts to.
type platforms struct {
	svc        *service
	admin      *browser
	provider   *fakeProvider
	gitlab     *fakeGitLabAPI
	mattermost *fakeMattermost
}

// startWithPlatforms starts a service on an empty database with keys as
// given, and sets it up with ada as the first administrator.
func startWithPlatforms(t *testing.T, cfg serviceConfig) platforms {
	t.Helper()
	freshDatabase(t)
	mattermost := newFakeMattermost(t)
	cfg.mattermostURL = mattermost.url()
	svc := startService(t, cfg)
	provider := newFakeProvider(t)
	return platforms{
		svc:        svc,
		admin:      setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada),
		provider:   provider,
		gitlab:     newFakeGitLabAPI(t),
		mattermost: mattermost,
	}
}

// connectGitLab creates a GitLab connection with the suite's access token.
func (p platforms) connectGitLab(t *testing.T) *apiv1.Connection {
	t.Helper()
	connection, err := p.admin.api(t).CreateConnection(context.Background(), &apiv1.ConnectionRequest{
		Kind:       apiv1.ConnectionKindGitlab,
		Name:       "gitlab",
		BaseURL:    p.gitlab.url(),
		Credential: apiv1.NewOptString(gitlabToken),
	})
	require.NoError(t, err)
	return connection
}

// connectMattermost creates a Mattermost connection with the bot's token.
func (p platforms) connectMattermost(t *testing.T) *apiv1.Connection {
	t.Helper()
	connection, err := p.admin.api(t).CreateConnection(context.Background(), &apiv1.ConnectionRequest{
		Kind:       apiv1.ConnectionKindMattermost,
		Name:       "chat",
		BaseURL:    p.mattermost.url(),
		Credential: apiv1.NewOptString(botToken),
	})
	require.NoError(t, err)
	return connection
}

// addSigningToken adds the suite's signing token to a forge connection.
func (p platforms) addSigningToken(t *testing.T, connection *apiv1.Connection) *apiv1.SigningToken {
	t.Helper()
	token, err := p.admin.api(t).AddSigningToken(context.Background(),
		&apiv1.SigningTokenRequest{Label: "group hook", Token: signingSecret},
		apiv1.AddSigningTokenParams{ID: connection.ID})
	require.NoError(t, err)
	return token
}

// An administrator connects a forge and a chat platform. PatchCaster shows
// whom each credential acts as, and where the forge sends its webhooks
// (SCEN-014).
func TestAnAdministratorConnectsGitLabAndMattermost(t *testing.T) {
	// arrange
	ctx := context.Background()
	p := startWithPlatforms(t, serviceConfig{})

	// act
	gitlab := p.connectGitLab(t)
	chat := p.connectMattermost(t)
	token := p.addSigningToken(t, gitlab)
	tokens, tokensErr := p.admin.api(t).ListSigningTokens(ctx, apiv1.ListSigningTokensParams{ID: gitlab.ID})
	listed, listErr := p.admin.api(t).ListConnections(ctx, apiv1.ListConnectionsParams{})

	// assert
	assert.Equal(t, gitlabUsername, gitlab.Account)
	assert.True(t, gitlab.HasCredential)
	assert.Equal(t, "http://"+p.svc.publicAddr+"/webhooks/"+strconv.FormatInt(gitlab.ID, 10), gitlab.WebhookURL.Or(""))
	assert.Equal(t, botUsername, chat.Account)
	assert.False(t, chat.WebhookURL.IsSet(), "a chat connection receives no webhooks")
	require.NoError(t, tokensErr)
	require.Len(t, tokens.Items, 1)
	assert.Equal(t, token.ID, tokens.Items[0].ID)
	require.NoError(t, listErr)
	assert.Len(t, listed.Items, 2)
	assert.Subset(t, auditActions(t, p.admin), []string{"connection.create", "signing_token.add"})
}

// A credential the platform refuses is not stored, and the refusal carries
// the platform's answer (SCEN-014).
func TestARefusedCredentialIsNotStored(t *testing.T) {
	// arrange
	ctx := context.Background()
	p := startWithPlatforms(t, serviceConfig{})

	// act
	_, err := p.admin.api(t).CreateConnection(ctx, &apiv1.ConnectionRequest{
		Kind:       apiv1.ConnectionKindGitlab,
		Name:       "gitlab",
		BaseURL:    p.gitlab.url(),
		Credential: apiv1.NewOptString("not-the-token"),
	})
	listed, listErr := p.admin.api(t).ListConnections(ctx, apiv1.ListConnectionsParams{})

	// assert
	problem := requireProblem(t, err, http.StatusUnprocessableEntity)
	require.Len(t, problem.Errors, 1)
	assert.Equal(t, "credential", problem.Errors[0].Path)
	assert.Equal(t, []string{"gitlab refused it: 401 Unauthorized"}, problem.Errors[0].Messages)
	require.NoError(t, listErr)
	assert.Empty(t, listed.Items)
}

// Neither a backup nor the logs reveal a connection's credential or signing
// token (SYSREQ-042).
func TestConnectionSecretsStaySealedAtRest(t *testing.T) {
	// arrange
	p := startWithPlatforms(t, serviceConfig{})
	p.addSigningToken(t, p.connectGitLab(t))
	p.connectMattermost(t)

	// act
	dump := dumpDatabase(t)
	logs := p.svc.logs.String()

	// assert
	require.Contains(t, dump, "connections", "the dump holds the connections")
	for _, secret := range []string{gitlabToken, botToken, signingSecret} {
		assert.NotContains(t, dump, secret)
		assert.NotContains(t, dump, hex.EncodeToString([]byte(secret)), "nor as bytea")
		assert.NotContains(t, logs, secret)
	}
}

// After a key rotation and a reseal, the new key alone opens every
// connection secret (ADR-035).
func TestConnectionSecretsSurviveAKeyRotation(t *testing.T) {
	// arrange
	ctx := context.Background()
	p := startWithPlatforms(t, serviceConfig{})
	gitlab := p.connectGitLab(t)
	p.addSigningToken(t, gitlab)
	p.connectMattermost(t)
	p.svc.stop(t)

	// act
	resealOut, resealCode := runPatchcaster(t, "secrets", "reseal",
		"--database-url", databaseURL,
		"--encryption-key", rotatedKey,
		"--previous-encryption-key", suiteKey)
	after := startService(t, serviceConfig{mattermostURL: p.mattermost.url(), encryptionKey: rotatedKey})
	admin := logInAs(t, after, p.provider, ada)
	moved := newFakeGitLabAPI(t)
	_, moveErr := admin.api(t).UpdateConnection(ctx,
		&apiv1.ConnectionUpdate{Name: "gitlab", BaseURL: moved.url()},
		apiv1.UpdateConnectionParams{ID: gitlab.ID, IfMatch: gitlab.Etag})

	// assert
	require.Zero(t, resealCode, resealOut)
	assert.Contains(t, resealOut, "count=5", "the key check, the client secret, two credentials and a signing token")
	assert.NoError(t, moveErr, "the stored credential opens with the new key and checks out at the new address")
}
