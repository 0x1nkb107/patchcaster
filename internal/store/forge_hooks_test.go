package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

var (
	hookActor   = store.Actor{Label: "hook registration"}
	groupScope  = store.HookScope{Kind: "group", Path: "payments"}
	allEvents   = store.HookEvents{MergeRequests: true, Pushes: true, Comments: true, Pipelines: true}
	hookAddress = "https://hooks.example.org/webhooks/1"
)

// registerHook stores a hook of scope that the forge registered with id.
func registerHook(t *testing.T, st *store.Postgres, forge *store.Connection, scope store.HookScope, id int64) int64 {
	t.Helper()
	ctx := context.Background()
	tokenID, err := st.ReserveSigningTokenID(ctx)
	require.NoError(t, err)
	err = st.RecordHookRegistered(ctx, hookActor, store.NewForgeHook{
		ConnectionID: forge.ID,
		Scope:        scope,
		Events:       allEvents,
		URL:          hookAddress,
		ForgeHookID:  id,
		Token: store.NewSigningToken{
			ID:           tokenID,
			ConnectionID: forge.ID,
			Label:        "registered hook",
			Sealed:       []byte("sealed"),
		},
	}, at(0))
	require.NoError(t, err)
	return tokenID
}

func TestARegisteredHookKeepsItsSigningToken(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", []byte("sealed"))

	// act
	tokenID := registerHook(t, st, forge, groupScope, 17)
	hooks, err := st.ForgeHooks(ctx, forge.ID)
	tokens, tokensErr := st.SigningTokens(ctx, forge.ID, 0, 10)

	// assert
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Equal(t, groupScope, hooks[0].Scope)
	assert.Equal(t, allEvents, hooks[0].Events)
	assert.Equal(t, int64(17), hooks[0].ForgeHookID)
	assert.Equal(t, tokenID, hooks[0].SigningTokenID)
	requireSameInstant(t, at(0), hooks[0].RegisteredAt, "registered at")
	require.NoError(t, tokensErr)
	require.Len(t, tokens, 1, "the hook's token verifies its deliveries")
	assert.Equal(t, "forge_hook.register", auditActions(t, st)[0])
}

func TestAFailureIsKeptUntilTheNextSuccess(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", []byte("sealed"))
	wanted := store.HookEvents{Comments: true, Pipelines: true}

	// act: the first attempt fails, the second succeeds.
	require.NoError(t, st.RecordHookFailed(ctx, forge.ID, groupScope, wanted, hookAddress, "gitlab answered http 404: 404 Group Not Found", at(0)))
	failed, failedErr := st.ForgeHooks(ctx, forge.ID)
	registerHook(t, st, forge, groupScope, 17)
	registered, registeredErr := st.ForgeHooks(ctx, forge.ID)

	// assert
	require.NoError(t, failedErr)
	require.Len(t, failed, 1)
	assert.Zero(t, failed[0].ForgeHookID, "nothing is registered yet")
	assert.Equal(t, wanted, failed[0].Events, "the row holds the hook the scope is to get")
	assert.Equal(t, "gitlab answered http 404: 404 Group Not Found", failed[0].LastError)
	require.NoError(t, registeredErr)
	assert.Empty(t, registered[0].LastError)
	assert.Equal(t, int64(17), registered[0].ForgeHookID)
}

func TestAFailedChangeKeepsWhatTheForgeHolds(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", []byte("sealed"))
	registerHook(t, st, forge, groupScope, 17)

	// act
	require.NoError(t, st.RecordHookFailed(ctx, forge.ID, groupScope, store.HookEvents{Comments: true}, "https://elsewhere.example", "gitlab answered http 500", at(1)))
	hooks, err := st.ForgeHooks(ctx, forge.ID)

	// assert
	require.NoError(t, err)
	assert.Equal(t, allEvents, hooks[0].Events)
	assert.Equal(t, hookAddress, hooks[0].URL)
	assert.Equal(t, "gitlab answered http 500", hooks[0].LastError)
}

func TestAChangeAppliesToRegisteredHooksOnly(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", []byte("sealed"))
	registerHook(t, st, forge, groupScope, 17)
	narrow := store.HookEvents{Comments: true, Pipelines: true}

	// act
	err := st.RecordHookChanged(ctx, hookActor, forge.ID, groupScope, narrow, "https://new.example/webhooks/1", at(1))
	unknownErr := st.RecordHookChanged(ctx, hookActor, forge.ID, store.HookScope{Kind: "repository", Path: "a/b"}, narrow, "x", at(1))
	hooks, listErr := st.ForgeHooks(ctx, forge.ID)

	// assert
	require.NoError(t, err)
	assert.ErrorIs(t, unknownErr, store.ErrNotFound)
	require.NoError(t, listErr)
	assert.Equal(t, narrow, hooks[0].Events)
	assert.Equal(t, "https://new.example/webhooks/1", hooks[0].URL)
	assert.Equal(t, "forge_hook.change", auditActions(t, st)[0])
}

func TestAForgottenHookTakesItsTokenAlong(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", []byte("sealed"))
	registerHook(t, st, forge, groupScope, 17)

	// act
	err := st.ForgetHook(ctx, hookActor, forge.ID, groupScope, at(1))
	againErr := st.ForgetHook(ctx, hookActor, forge.ID, groupScope, at(1))
	hooks, hooksErr := st.ForgeHooks(ctx, forge.ID)
	tokens, tokensErr := st.SigningTokens(ctx, forge.ID, 0, 10)

	// assert
	require.NoError(t, err)
	require.NoError(t, againErr, "forgetting twice is no error")
	require.NoError(t, hooksErr)
	assert.Empty(t, hooks)
	require.NoError(t, tokensErr)
	assert.Empty(t, tokens)
	assert.Equal(t, "forge_hook.remove", auditActions(t, st)[0])
}

func TestAHookKeepsItsConnectionAndToken(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", []byte("sealed"))
	tokenID := registerHook(t, st, forge, groupScope, 17)

	// act
	tokenErr := st.RemoveSigningToken(ctx, routingActor, forge.ID, tokenID, at(1))
	connectionErr := st.DeleteConnection(ctx, routingActor, forge.ID, forge.Version, at(1))

	// assert
	assert.ErrorIs(t, tokenErr, store.ErrInUse, "the hook's deliveries need the token")
	assert.ErrorIs(t, connectionErr, store.ErrInUse, "the hook would stay behind at the forge")
}

func TestOnlyOneReplicaRegistersAConnectionsHooks(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()

	// act
	release, locked, err := st.LockHooks(ctx, 1)
	require.NoError(t, err)
	_, lockedAgain, againErr := st.LockHooks(ctx, 1)
	releaseOther, lockedOther, otherErr := st.LockHooks(ctx, 2)
	release()
	releaseAfter, lockedAfter, afterErr := st.LockHooks(ctx, 1)

	// assert
	assert.True(t, locked)
	require.NoError(t, againErr)
	assert.False(t, lockedAgain, "the connection is taken")
	require.NoError(t, otherErr)
	assert.True(t, lockedOther, "another connection is free")
	releaseOther()
	require.NoError(t, afterErr)
	assert.True(t, lockedAfter, "a released lock is free again")
	releaseAfter()
}

func TestSourceScopesNameEverySourceOfAConnection(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	forge := createConnection(t, st, "gitlab", "gitlab", nil)
	other := createConnection(t, st, "gitlab", "other", nil)
	group := createSource(t, st, forge, "payments", "group", "payments")
	repo := createSource(t, st, forge, "api", "repository", "payments/api")
	createSource(t, st, other, "elsewhere", "group", "elsewhere")

	// act
	scopes, err := st.SourceScopes(ctx, forge.ID)

	// assert
	require.NoError(t, err)
	assert.Equal(t, []store.SourceScope{
		{SourceID: group.ID, Scope: store.HookScope{Kind: "group", Path: "payments"}},
		{SourceID: repo.ID, Scope: store.HookScope{Kind: "repository", Path: "payments/api"}},
	}, scopes)
}
