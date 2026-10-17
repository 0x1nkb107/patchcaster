package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// login is what the test provider says about subject.
func login(subject string) store.Login {
	return store.Login{
		Issuer:      "https://idp.example",
		Subject:     subject,
		DisplayName: subject + " name",
		Email:       subject + "@example.com",
	}
}

// completeSetup makes subject the first administrator through a setup link.
func completeSetup(t *testing.T, st *store.Postgres, subject string) *store.Person {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	link := []byte("hash-of-the-setup-link-for-" + subject)
	require.NoError(t, st.CreateSetupLink(ctx, store.Actor{Label: "bootstrap"}, link, now, now.Add(time.Hour)))
	admin, err := st.CompleteSetup(ctx, link, login(subject), now)
	require.NoError(t, err)
	return admin
}

// auditActions lists the actions of the newest audit entries, newest first.
func auditActions(t *testing.T, st *store.Postgres) []string {
	t.Helper()
	entries, err := st.AuditEntries(context.Background(), 0, 100)
	require.NoError(t, err)
	actions := make([]string, 0, len(entries))
	for _, entry := range entries {
		actions = append(actions, entry.Action)
	}
	return actions
}

func TestTheFirstLoginCreatesAUser(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// act
	first, err := st.RecordLogin(ctx, login("ada"), now)
	require.NoError(t, err)
	renamed := login("ada")
	renamed.DisplayName = "Ada L."
	again, err := st.RecordLogin(ctx, renamed, now)
	require.NoError(t, err)

	// assert
	assert.False(t, first.Administrator, "people start with the role User")
	assert.Equal(t, first.ID, again.ID, "issuer and subject identify a person")
	assert.Equal(t, "Ada L.", again.DisplayName)
	assert.Empty(t, auditActions(t, st), "only an administrator's login is audited")
}

func TestTheLastAdministratorStays(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	admin := completeSetup(t, st, "ada")

	// act
	_, demoteErr := st.SetAdministrator(context.Background(), store.ActorOf(admin), admin.ID, false, admin.Version, time.Now())

	// assert
	assert.ErrorIs(t, demoteErr, store.ErrLastAdministrator)
}

func TestARoleChangeNeedsTheCurrentVersion(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	user, err := st.RecordLogin(ctx, login("bob"), now)
	require.NoError(t, err)
	actor := store.Actor{Label: "test"}

	// act
	promoted, err := st.SetAdministrator(ctx, actor, user.ID, true, user.Version, now)
	require.NoError(t, err)
	_, staleErr := st.SetAdministrator(ctx, actor, user.ID, false, user.Version, now)
	stored, readErr := st.Person(ctx, user.ID)

	// assert
	require.NoError(t, readErr)
	assert.True(t, promoted.Administrator)
	assert.Equal(t, user.Version+1, promoted.Version)
	assert.Equal(t, promoted.Version, stored.Version, "the returned version is the stored one")
	assert.ErrorIs(t, staleErr, store.ErrVersionMismatch, "a change at an old version must not win")
}

// A change that fails leaves no audit entry behind (SYSREQ-041).
func TestAFailedChangeIsNotAudited(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	admin := completeSetup(t, st, "ada")
	before := auditActions(t, st)

	// act
	_, staleErr := st.SetAdministrator(context.Background(), store.ActorOf(admin), admin.ID, true, admin.Version-1, time.Now())

	// assert
	require.ErrorIs(t, staleErr, store.ErrVersionMismatch)
	assert.Equal(t, before, auditActions(t, st))
}

func TestTheRecoveryGrantCreatesAnAdministrator(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// act
	err := st.GrantAdministrator(ctx, store.Actor{Label: "recovery command"}, "https://idp.example", "carol", now)
	person, loginErr := st.RecordLogin(ctx, login("carol"), now)

	// assert
	require.NoError(t, err)
	require.NoError(t, loginErr)
	assert.True(t, person.Administrator, "the named person is an administrator at their first login")
	assert.Equal(t, []string{"administrator.login", "person.recovery_grant"}, auditActions(t, st))
}

func TestARefusedLoginIsAudited(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)

	// act
	err := st.RecordRefusedLogin(context.Background(), login("mallory"), "not a member of a group that may log in", time.Now())
	entries, listErr := st.AuditEntries(context.Background(), 0, 1)

	// assert
	require.NoError(t, err)
	require.NoError(t, listErr)
	require.Len(t, entries, 1)
	assert.Equal(t, "login.refused", entries[0].Action)
	assert.Contains(t, string(entries[0].After), "not a member of a group")
}
