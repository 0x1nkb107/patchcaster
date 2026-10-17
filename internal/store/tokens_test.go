package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// createToken stores a token named name for person, which expires after
// lifetime.
func createToken(t *testing.T, st *store.Postgres, person *store.Person, name string, lifetime time.Duration) *store.APIToken {
	t.Helper()
	now := time.Now().UTC()
	token, err := st.CreateToken(context.Background(), store.ActorOf(person), store.NewAPIToken{
		PersonID:  person.ID,
		Name:      name,
		Hash:      []byte("hash-of-" + name),
		ExpiresAt: now.Add(lifetime),
	}, now)
	require.NoError(t, err)
	return token
}

func TestATokenAuthenticatesUntilItExpires(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	ada := completeSetup(t, st, "ada")
	token := createToken(t, st, ada, "ci", time.Hour)

	// act
	found, liveErr := st.AuthenticateToken(ctx, []byte("hash-of-ci"), time.Now())
	_, laterErr := st.AuthenticateToken(ctx, []byte("hash-of-ci"), time.Now().Add(2*time.Hour))
	_, unknownErr := st.AuthenticateToken(ctx, []byte("hash-of-nothing"), time.Now())

	// assert
	require.NoError(t, liveErr)
	assert.Equal(t, token.ID, found.ID)
	assert.Equal(t, ada.ID, found.PersonID)
	assert.ErrorIs(t, laterErr, store.ErrNotFound, "an expired token authenticates nobody")
	assert.ErrorIs(t, unknownErr, store.ErrNotFound)
	assert.Contains(t, auditActions(t, st), "token.create")
}

func TestARevokedTokenAuthenticatesNobody(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	ada := completeSetup(t, st, "ada")
	token := createToken(t, st, ada, "ci", time.Hour)

	// act
	revokeErr := st.RevokeToken(ctx, store.ActorOf(ada), ada.ID, token.ID, time.Now())
	againErr := st.RevokeToken(ctx, store.ActorOf(ada), ada.ID, token.ID, time.Now())
	_, authErr := st.AuthenticateToken(ctx, []byte("hash-of-ci"), time.Now())
	listed, listErr := st.Tokens(ctx, ada.ID, 0, 10)

	// assert
	require.NoError(t, revokeErr)
	require.NoError(t, againErr, "revoking twice is no error")
	assert.ErrorIs(t, authErr, store.ErrNotFound)
	require.NoError(t, listErr)
	require.Len(t, listed, 1, "a revoked token stays listed")
	assert.False(t, listed[0].RevokedAt.IsZero())
	actions := auditActions(t, st)
	assert.Equal(t, "token.revoke", actions[0])
	assert.NotEqual(t, "token.revoke", actions[1], "only the first revocation is recorded")
}

func TestOnlyTheOwnerRevokesAToken(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	ada := completeSetup(t, st, "ada")
	bob, err := st.RecordLogin(ctx, login("bob"), time.Now())
	require.NoError(t, err)
	token := createToken(t, st, ada, "ci", time.Hour)

	// act
	err = st.RevokeToken(ctx, store.ActorOf(bob), bob.ID, token.ID, time.Now())
	_, authErr := st.AuthenticateToken(ctx, []byte("hash-of-ci"), time.Now())

	// assert
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.NoError(t, authErr, "the token still works")
}

func TestRevokingAccessEndsTokensAndSessions(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	ada := completeSetup(t, st, "ada")
	bob, err := st.RecordLogin(ctx, login("bob"), time.Now())
	require.NoError(t, err)
	createToken(t, st, bob, "laptop", time.Hour)
	createToken(t, st, bob, "ci", time.Hour)
	adasToken := createToken(t, st, ada, "ada-ci", time.Hour)

	// act
	revoked, revokeErr := st.RevokeAccess(ctx, store.ActorOf(ada), bob.ID, time.Now())
	after, readErr := st.Person(ctx, bob.ID)
	_, bobsErr := st.AuthenticateToken(ctx, []byte("hash-of-ci"), time.Now())
	adas, adasErr := st.AuthenticateToken(ctx, []byte("hash-of-ada-ci"), time.Now())
	_, unknownErr := st.RevokeAccess(ctx, store.ActorOf(ada), 9999, time.Now())

	// assert
	require.NoError(t, revokeErr)
	assert.Equal(t, 2, revoked)
	require.NoError(t, readErr)
	assert.Equal(t, bob.SessionEpoch+1, after.SessionEpoch, "every earlier session ends")
	assert.Equal(t, bob.Version, after.Version, "the person's settings did not change")
	assert.ErrorIs(t, bobsErr, store.ErrNotFound)
	require.NoError(t, adasErr, "other people's tokens stay")
	assert.Equal(t, adasToken.ID, adas.ID)
	assert.ErrorIs(t, unknownErr, store.ErrNotFound)
	assert.Equal(t, "person.access_revoked", auditActions(t, st)[0])
}

func TestAuditEntriesNameTheToken(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	ada := completeSetup(t, st, "ada")
	token := createToken(t, st, ada, "ci", time.Hour)
	bob, err := st.RecordLogin(ctx, login("bob"), time.Now())
	require.NoError(t, err)
	actor := store.ActorOf(ada)
	actor.TokenID = token.ID
	actor.TokenName = token.Name

	// act
	_, err = st.SetAdministrator(ctx, actor, bob.ID, true, bob.Version, time.Now())
	entries, listErr := st.AuditEntries(ctx, 0, 1)

	// assert
	require.NoError(t, err)
	require.NoError(t, listErr)
	require.Len(t, entries, 1)
	assert.Equal(t, ada.ID, entries[0].ActorPersonID)
	assert.Equal(t, token.ID, entries[0].ActorTokenID)
	assert.Equal(t, "ci", entries[0].ActorTokenName)
}
