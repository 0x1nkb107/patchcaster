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

// testSettings is a provider as the setup stores it.
func testSettings() store.LoginSettings {
	return store.LoginSettings{
		Issuer:       "https://idp.example",
		ClientID:     "patchcaster",
		ClientSecret: []byte("sealed-secret"),
	}
}

func TestSetupMakesTheFirstAdministratorOnce(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	link := []byte("hash-of-the-setup-link")
	require.NoError(t, st.CreateSetupLink(ctx, store.Actor{Label: "bootstrap"}, link, now, now.Add(time.Hour)))

	// act
	admin, firstErr := st.CompleteSetup(ctx, link, login("ada"), now)
	_, secondErr := st.CompleteSetup(ctx, link, login("eve"), now)
	admins, _ := st.AdministratorCount(ctx)
	stored, readErr := st.Person(ctx, admin.ID)

	// assert
	require.NoError(t, firstErr)
	require.NoError(t, readErr)
	assert.True(t, admin.Administrator)
	assert.Equal(t, stored.Version, admin.Version, "the returned version is the stored one")
	assert.ErrorIs(t, secondErr, store.ErrSetupLinkSpent, "a setup link works once")
	assert.Equal(t, 1, admins)
	assert.Equal(t, []string{"setup.complete", "setup.link_issued"}, auditActions(t, st))
}

func TestAnExpiredSetupLinkIsSpent(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	link := []byte("hash-of-an-old-link")
	require.NoError(t, st.CreateSetupLink(ctx, store.Actor{Label: "bootstrap"}, link, now.Add(-2*time.Hour), now.Add(-time.Hour)))

	// act
	_, err := st.CompleteSetup(ctx, link, login("ada"), now)

	// assert
	assert.ErrorIs(t, err, store.ErrSetupLinkSpent)
}

func TestNoSetupLinkOnceAnAdministratorExists(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	completeSetup(t, st, "ada")
	now := time.Now().UTC()

	// act
	err := st.CreateSetupLink(context.Background(), store.Actor{Label: "bootstrap"}, []byte("late"), now, now.Add(time.Hour))

	// assert
	assert.ErrorIs(t, err, store.ErrAdministratorExists)
}

func TestSetupSettingsNeedAValidLink(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)

	// act
	err := st.PutSetupLoginSettings(context.Background(), store.Actor{Label: "setup"}, []byte("no-such-link"), testSettings(), time.Now())

	// assert
	assert.ErrorIs(t, err, store.ErrSetupLinkSpent)
}

// A link issued before the setup completed cannot change the provider after
// it.
func TestSetupSettingsCloseWithTheSetup(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	spare := []byte("hash-of-a-spare-link")
	require.NoError(t, st.CreateSetupLink(ctx, store.Actor{Label: "bootstrap"}, spare, now, now.Add(time.Hour)))
	completeSetup(t, st, "ada")

	// act
	err := st.PutSetupLoginSettings(ctx, store.Actor{Label: "setup"}, spare, testSettings(), now)

	// assert
	assert.ErrorIs(t, err, store.ErrAdministratorExists)
}

func TestSetupSettingsAreAuditedWithoutTheSecret(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	link := []byte("hash-of-the-setup-link")
	require.NoError(t, st.CreateSetupLink(ctx, store.Actor{Label: "bootstrap"}, link, now, now.Add(time.Hour)))

	// act
	err := st.PutSetupLoginSettings(ctx, store.Actor{Label: "setup"}, link, testSettings(), now)
	entries, listErr := st.AuditEntries(ctx, 0, 1)

	// assert
	require.NoError(t, err)
	require.NoError(t, listErr)
	require.Len(t, entries, 1)
	assert.Equal(t, "login_settings.setup", entries[0].Action)
	var after map[string]any
	require.NoError(t, json.Unmarshal(entries[0].After, &after))
	assert.Equal(t, "secret changed", after["client_secret"])
	assert.Equal(t, []any{}, after["allowed_groups"])
	assert.NotContains(t, string(entries[0].After), "sealed-secret")
}
