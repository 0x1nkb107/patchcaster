package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

var connectionActor = store.Actor{Label: "ada"}

// createConnection stores a connection named name of kind, sealed with a
// stand-in credential unless credential is nil.
func createConnection(t *testing.T, st *store.Postgres, kind, name string, credential []byte) *store.Connection {
	t.Helper()
	ctx := context.Background()
	id, err := st.ReserveConnectionID(ctx)
	require.NoError(t, err)
	created, err := st.CreateConnection(ctx, connectionActor, store.Connection{
		ID:         id,
		Kind:       kind,
		Name:       name,
		BaseURL:    "https://" + name + ".example.org",
		Credential: credential,
		Account:    "patchcaster",
	}, time.Now())
	require.NoError(t, err)
	return created
}

// addSigningToken adds a stand-in sealed signing token to connection.
func addSigningToken(t *testing.T, st *store.Postgres, connection *store.Connection, label string) *store.SigningToken {
	t.Helper()
	ctx := context.Background()
	id, err := st.ReserveSigningTokenID(ctx)
	require.NoError(t, err)
	token, err := st.AddSigningToken(ctx, connectionActor, store.NewSigningToken{
		ID:           id,
		ConnectionID: connection.ID,
		Label:        label,
		Sealed:       []byte("sealed-" + label),
	}, time.Now())
	require.NoError(t, err)
	return token
}

// lastAudit returns the newest audit entry, decoded.
func lastAudit(t *testing.T, st *store.Postgres) (store.AuditEntry, map[string]any, map[string]any) {
	t.Helper()
	entries, err := st.AuditEntries(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	var before, after map[string]any
	require.NoError(t, json.Unmarshal(entries[0].Before, &before))
	require.NoError(t, json.Unmarshal(entries[0].After, &after))
	return entries[0], before, after
}

func TestAConnectionIsStoredUnderItsReservedID(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()

	// act
	created := createConnection(t, st, "gitlab", "gitlab", []byte("sealed-credential"))
	read, readErr := st.Connection(ctx, created.ID)
	listed, listErr := st.Connections(ctx, 0, 10)
	entry, _, after := lastAudit(t, st)

	// assert
	require.NoError(t, readErr)
	require.NoError(t, listErr)
	assert.Equal(t, created, read)
	assert.Len(t, listed, 1)
	assert.Equal(t, "connection.create", entry.Action)
	assert.Equal(t, "secret changed", after["credential"])
	assert.NotContains(t, string(entry.After), "sealed-credential")
}

func TestConnectionNamesAreUnique(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	createConnection(t, st, "gitlab", "gitlab", nil)
	other := createConnection(t, st, "mattermost", "chat", []byte("sealed"))
	id, err := st.ReserveConnectionID(ctx)
	require.NoError(t, err)

	// act
	_, createErr := st.CreateConnection(ctx, connectionActor, store.Connection{ID: id, Kind: "gitlab", Name: "gitlab", BaseURL: "https://x"}, time.Now())
	renamed := *other
	renamed.Name = "gitlab"
	renamed.Credential = nil
	_, updateErr := st.UpdateConnection(ctx, connectionActor, renamed, time.Now())

	// assert
	assert.ErrorIs(t, createErr, store.ErrNameTaken)
	assert.ErrorIs(t, updateErr, store.ErrNameTaken)
}

func TestAConnectionUpdateKeepsTheCredentialUnlessGivenOne(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	created := createConnection(t, st, "mattermost", "chat", []byte("sealed-first"))

	// act
	renamed := *created
	renamed.Name = "chat-renamed"
	renamed.Credential = nil
	kept, keepErr := st.UpdateConnection(ctx, connectionActor, renamed, time.Now())
	_, _, keptAudit := lastAudit(t, st)
	replaced := *kept
	replaced.Credential = []byte("sealed-second")
	afterReplace, replaceErr := st.UpdateConnection(ctx, connectionActor, replaced, time.Now())
	_, _, replacedAudit := lastAudit(t, st)
	_, staleErr := st.UpdateConnection(ctx, connectionActor, renamed, time.Now())

	// assert
	require.NoError(t, keepErr)
	assert.Equal(t, []byte("sealed-first"), kept.Credential)
	assert.Equal(t, created.Version+1, kept.Version)
	assert.Nil(t, keptAudit["credential"], "a kept credential is not reported as changed")
	require.NoError(t, replaceErr)
	assert.Equal(t, []byte("sealed-second"), afterReplace.Credential)
	assert.Equal(t, "secret changed", replacedAudit["credential"])
	assert.ErrorIs(t, staleErr, store.ErrVersionMismatch)
}

func TestDeletingAConnectionRemovesItsSigningTokens(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	created := createConnection(t, st, "gitlab", "gitlab", nil)
	addSigningToken(t, st, created, "group hook")

	// act
	staleErr := st.DeleteConnection(ctx, connectionActor, created.ID, created.Version+1, time.Now())
	err := st.DeleteConnection(ctx, connectionActor, created.ID, created.Version, time.Now())
	_, readErr := st.Connection(ctx, created.ID)
	sealed, listErr := st.SealedSigningTokens(ctx)
	entry, before, _ := lastAudit(t, st)

	// assert
	assert.ErrorIs(t, staleErr, store.ErrVersionMismatch)
	require.NoError(t, err)
	assert.ErrorIs(t, readErr, store.ErrNotFound)
	require.NoError(t, listErr)
	assert.Empty(t, sealed)
	assert.Equal(t, "connection.delete", entry.Action)
	assert.Equal(t, "gitlab", before["name"])
}

func TestSigningTokensAreListedWithoutTheirValues(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	gitlab := createConnection(t, st, "gitlab", "gitlab", nil)
	other := createConnection(t, st, "gitlab", "other", nil)
	first := addSigningToken(t, st, gitlab, "group hook")
	addSigningToken(t, st, gitlab, "project hook")
	othersToken := addSigningToken(t, st, other, "elsewhere")

	// act
	listed, listErr := st.SigningTokens(ctx, gitlab.ID, 0, 10)
	removeErr := st.RemoveSigningToken(ctx, connectionActor, gitlab.ID, first.ID, time.Now())
	entry, _, _ := lastAudit(t, st)
	againErr := st.RemoveSigningToken(ctx, connectionActor, gitlab.ID, first.ID, time.Now())
	wrongErr := st.RemoveSigningToken(ctx, connectionActor, gitlab.ID, othersToken.ID, time.Now())
	after, _ := st.SigningTokens(ctx, gitlab.ID, 0, 10)

	// assert
	require.NoError(t, listErr)
	require.Len(t, listed, 2)
	assert.Equal(t, "group hook", listed[0].Label)
	require.NoError(t, removeErr)
	assert.Equal(t, "signing_token.remove", entry.Action)
	assert.NotContains(t, string(entry.Before), "sealed-")
	assert.ErrorIs(t, againErr, store.ErrNotFound)
	assert.ErrorIs(t, wrongErr, store.ErrNotFound, "a token of another connection is unknown here")
	require.Len(t, after, 1)
	assert.Equal(t, "project hook", after[0].Label)
}

func TestConnectionSecretsReseal(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	chat := createConnection(t, st, "mattermost", "chat", []byte("sealed-credential"))
	createConnection(t, st, "gitlab", "no-api", nil)
	token := addSigningToken(t, st, chat, "hook")

	// act
	credentials, credErr := st.ConnectionCredentials(ctx)
	staleReplaced, _ := st.ReplaceConnectionCredential(ctx, chat.ID, []byte("not-stored"), []byte("resealed"))
	replaced, replaceErr := st.ReplaceConnectionCredential(ctx, chat.ID, []byte("sealed-credential"), []byte("resealed"))
	tokenReplaced, tokenErr := st.ReplaceSigningToken(ctx, token.ID, []byte("sealed-hook"), []byte("resealed-hook"))
	after, _ := st.Connection(ctx, chat.ID)
	tokens, _ := st.SealedSigningTokens(ctx)

	// assert
	require.NoError(t, credErr)
	require.Len(t, credentials, 1, "a connection without a credential has nothing to reseal")
	assert.Equal(t, chat.ID, credentials[0].ID)
	assert.False(t, staleReplaced)
	require.NoError(t, replaceErr)
	assert.True(t, replaced)
	assert.Equal(t, []byte("resealed"), after.Credential)
	assert.Equal(t, chat.Version, after.Version, "a reseal changes no setting")
	require.NoError(t, tokenErr)
	assert.True(t, tokenReplaced)
	assert.Equal(t, []byte("resealed-hook"), tokens[0].Sealed)
}
