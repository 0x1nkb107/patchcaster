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

// storeTestSettings runs a setup that stores testSettings and returns them
// as stored.
func storeTestSettings(t *testing.T, st *store.Postgres) *store.LoginSettings {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	link := []byte("hash-of-the-setup-link")
	require.NoError(t, st.CreateSetupLink(ctx, store.Actor{Label: "bootstrap"}, link, now, now.Add(time.Hour)))
	require.NoError(t, st.PutSetupLoginSettings(ctx, store.Actor{Label: "setup"}, link, testSettings(), now))
	stored, err := st.LoginSettings(ctx)
	require.NoError(t, err)
	return stored
}

func TestLoginSettingsKeepTheSecretUnlessGivenOne(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	stored := storeTestSettings(t, st)
	actor := store.Actor{Label: "ada"}

	// act
	kept := *stored
	kept.ClientSecret = nil
	kept.AllowedGroups = []string{"engineering"}
	_, keepErr := st.UpdateLoginSettings(ctx, actor, kept, time.Now())
	afterKeep, _ := st.LoginSettings(ctx)
	replaced := *afterKeep
	replaced.ClientSecret = []byte("another-sealed-secret")
	_, replaceErr := st.UpdateLoginSettings(ctx, actor, replaced, time.Now())
	afterReplace, _ := st.LoginSettings(ctx)

	// assert
	require.NoError(t, keepErr)
	require.NoError(t, replaceErr)
	assert.Equal(t, []byte("sealed-secret"), afterKeep.ClientSecret)
	assert.Equal(t, []string{"engineering"}, afterKeep.AllowedGroups)
	assert.Equal(t, []byte("another-sealed-secret"), afterReplace.ClientSecret)
}

func TestLoginSettingsChangesAreAuditedWithoutTheSecret(t *testing.T) {
	tests := []struct {
		name       string
		secret     []byte
		wantSecret any
	}{
		{"with a new secret", []byte("another-sealed-secret"), "secret changed"},
		{"with the secret kept", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			st, _ := openTestStore(t)
			ctx := context.Background()
			changed := *storeTestSettings(t, st)
			changed.ClientSecret = tc.secret
			changed.GroupsClaim = "groups"

			// act
			updated, updateErr := st.UpdateLoginSettings(ctx, store.Actor{Label: "ada"}, changed, time.Now())
			_, staleErr := st.UpdateLoginSettings(ctx, store.Actor{Label: "ada"}, changed, time.Now())
			entries, _ := st.AuditEntries(ctx, 0, 1)

			// assert
			require.NoError(t, updateErr)
			assert.Equal(t, "groups", updated.GroupsClaim)
			assert.Nil(t, updated.ClientSecret, "returned settings carry no secret")
			assert.ErrorIs(t, staleErr, store.ErrVersionMismatch)
			require.Len(t, entries, 1)
			assert.Equal(t, "login_settings.update", entries[0].Action)
			var after map[string]any
			require.NoError(t, json.Unmarshal(entries[0].After, &after))
			assert.Equal(t, tc.wantSecret, after["client_secret"])
			assert.NotContains(t, string(entries[0].Before)+string(entries[0].After), "sealed-secret")
		})
	}
}

func TestTheClientSecretReseals(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	stored := storeTestSettings(t, st)

	// act
	staleReplaced, staleErr := st.ReplaceLoginClientSecret(ctx, []byte("not-what-is-stored"), []byte("resealed"))
	replaced, err := st.ReplaceLoginClientSecret(ctx, stored.ClientSecret, []byte("resealed"))
	after, _ := st.LoginSettings(ctx)

	// assert
	require.NoError(t, staleErr)
	require.NoError(t, err)
	assert.False(t, staleReplaced, "a secret replaced meanwhile stays")
	assert.True(t, replaced)
	assert.Equal(t, []byte("resealed"), after.ClientSecret)
	assert.Equal(t, stored.Version, after.Version, "a reseal changes no setting")
}
