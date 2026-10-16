package main

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/seal"
)

// memoryKeyCheck is an in-memory key check store.
type memoryKeyCheck struct {
	sealed   []byte
	racedBy  []byte // stored by "another instance" just before the first init
	replaced int
}

func (m *memoryKeyCheck) KeyCheck(context.Context) ([]byte, error) { return m.sealed, nil }

func (m *memoryKeyCheck) InitKeyCheck(_ context.Context, sealed []byte, _ time.Time) error {
	if m.racedBy != nil {
		m.sealed, m.racedBy = m.racedBy, nil
		return nil
	}
	if m.sealed == nil {
		m.sealed = sealed
	}
	return nil
}

func (m *memoryKeyCheck) ReplaceKeyCheck(_ context.Context, sealed []byte, _ time.Time) error {
	m.sealed = sealed
	m.replaced++
	return nil
}

func encodedKey(b byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(b), seal.KeySize)))
}

func boxOf(t *testing.T, active string, previous ...string) *seal.Box {
	t.Helper()
	box, err := KeyFlags{EncryptionKey: active, PreviousEncryptionKeys: previous}.box()
	require.NoError(t, err)
	return box
}

func TestKeyFlagErrorsNameTheFlagNotTheKey(t *testing.T) {
	tests := []struct {
		name  string
		flags KeyFlags
		want  string
	}{
		{"a malformed active key", KeyFlags{EncryptionKey: "secret-but-wrong"}, "--encryption-key"},
		{"a short active key", KeyFlags{EncryptionKey: base64.StdEncoding.EncodeToString([]byte("short"))}, "--encryption-key"},
		{"a malformed previous key", KeyFlags{EncryptionKey: encodedKey('a'), PreviousEncryptionKeys: []string{encodedKey('b'), "secret-but-wrong"}}, "--previous-encryption-key 2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			_, err := tc.flags.box()

			// assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), "secret-but-wrong")
		})
	}
}

func TestTheFirstStartStoresTheKeyCheck(t *testing.T) {
	// arrange
	st := &memoryKeyCheck{}

	// act
	stale, err := verifyInstanceKey(context.Background(), st, boxOf(t, encodedKey('a')), time.Now())

	// assert
	require.NoError(t, err)
	assert.False(t, stale)
	assert.NotNil(t, st.sealed)
}

func TestAnotherKeyIsRefused(t *testing.T) {
	// arrange
	st := &memoryKeyCheck{}
	ctx := context.Background()
	_, err := verifyInstanceKey(ctx, st, boxOf(t, encodedKey('a')), time.Now())
	require.NoError(t, err)

	// act
	_, err = verifyInstanceKey(ctx, st, boxOf(t, encodedKey('b')), time.Now())

	// assert
	assert.ErrorIs(t, err, errWrongKey)
}

func TestAPreviousKeyOpensButAsksForAReseal(t *testing.T) {
	// arrange
	st := &memoryKeyCheck{}
	ctx := context.Background()
	_, err := verifyInstanceKey(ctx, st, boxOf(t, encodedKey('a')), time.Now())
	require.NoError(t, err)
	rotated := boxOf(t, encodedKey('b'), encodedKey('a'))

	// act
	staleBefore, verifyErr := verifyInstanceKey(ctx, st, rotated, time.Now())
	resealed, resealErr := resealSecrets(ctx, st, rotated, time.Now())
	staleAfter, _ := verifyInstanceKey(ctx, st, boxOf(t, encodedKey('b')), time.Now())

	// assert
	require.NoError(t, verifyErr)
	require.NoError(t, resealErr)
	assert.True(t, staleBefore, "secrets sealed by the previous key are stale")
	assert.Equal(t, 1, resealed)
	assert.False(t, staleAfter, "after the reseal the new key opens everything on its own")
}

func TestAResealWithNothingStaleChangesNothing(t *testing.T) {
	// arrange
	st := &memoryKeyCheck{}
	ctx := context.Background()
	box := boxOf(t, encodedKey('a'))
	_, err := verifyInstanceKey(ctx, st, box, time.Now())
	require.NoError(t, err)

	// act
	resealed, err := resealSecrets(ctx, st, box, time.Now())

	// assert
	require.NoError(t, err)
	assert.Zero(t, resealed)
	assert.Zero(t, st.replaced)
}

func TestARaceAtTheFirstStartKeepsTheWinner(t *testing.T) {
	// arrange: another instance with another key stored its check first.
	winner, err := boxOf(t, encodedKey('w')).Seal([]byte(keyCheckPlaintext), keyCheckBinding)
	require.NoError(t, err)
	st := &memoryKeyCheck{racedBy: winner}

	// act
	_, err = verifyInstanceKey(context.Background(), st, boxOf(t, encodedKey('a')), time.Now())

	// assert
	assert.ErrorIs(t, err, errWrongKey, "the loser must verify the winner's check, not its own")
	assert.Equal(t, winner, st.sealed)
}
