package seal_test

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/seal"
)

// Fixed test keys: 32 bytes of one value each, base64-encoded.
var (
	keyA = base64.StdEncoding.EncodeToString(repeat('a'))
	keyB = base64.StdEncoding.EncodeToString(repeat('b'))
)

func repeat(b byte) []byte {
	raw := make([]byte, seal.KeySize)
	for i := range raw {
		raw[i] = b
	}
	return raw
}

func mustKey(t *testing.T, encoded string) seal.Key {
	t.Helper()
	key, err := seal.ParseKey(encoded)
	require.NoError(t, err)
	return key
}

func TestSealedValuesOpenAtTheirBindingOnly(t *testing.T) {
	// arrange
	box := seal.NewBox(mustKey(t, keyA))
	sealed, err := box.Seal([]byte("bot-token"), "connections.credentials.1")
	require.NoError(t, err)

	// act
	opened, openErr := box.Open(sealed, "connections.credentials.1")
	_, movedErr := box.Open(sealed, "connections.credentials.2")

	// assert
	require.NoError(t, openErr)
	assert.Equal(t, "bot-token", string(opened))
	assert.ErrorIs(t, movedErr, seal.ErrTampered, "a value copied into another row must not open")
	assert.NotContains(t, string(sealed), "bot-token")
}

func TestARotationOpensOldValuesAndMarksThemStale(t *testing.T) {
	// arrange
	old := seal.NewBox(mustKey(t, keyA))
	sealed, err := old.Seal([]byte("secret"), "binding")
	require.NoError(t, err)
	rotated := seal.NewBox(mustKey(t, keyB), mustKey(t, keyA))

	// act
	opened, openErr := rotated.Open(sealed, "binding")
	staleBefore, _ := rotated.Stale(sealed)
	resealed, resealErr := rotated.Seal(opened, "binding")
	staleAfter, _ := rotated.Stale(resealed)

	// assert
	require.NoError(t, openErr)
	require.NoError(t, resealErr)
	assert.Equal(t, "secret", string(opened))
	assert.True(t, staleBefore, "a value sealed by the previous key is stale")
	assert.False(t, staleAfter, "a resealed value is current")
}

func TestOpenRejects(t *testing.T) {
	sealedByA, err := seal.NewBox(mustKey(t, keyA)).Seal([]byte("secret"), "binding")
	require.NoError(t, err)
	tampered := append([]byte(nil), sealedByA...)
	tampered[len(tampered)-1] ^= 0xff
	wrongVersion := append([]byte(nil), sealedByA...)
	wrongVersion[0] = 9

	tests := []struct {
		name   string
		sealed []byte
		want   error
	}{
		{"a value sealed by an unconfigured key", sealedByA, seal.ErrUnknownKey},
		{"a value too short to hold a header", []byte{1, 2, 3}, seal.ErrMalformed},
		{"a value of an unknown format version", wrongVersion, seal.ErrMalformed},
	}
	boxB := seal.NewBox(mustKey(t, keyB))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			_, err := boxB.Open(tc.sealed, "binding")

			// assert
			assert.ErrorIs(t, err, tc.want)
		})
	}
	t.Run("a changed ciphertext", func(t *testing.T) {
		// act
		_, err := seal.NewBox(mustKey(t, keyA)).Open(tampered, "binding")

		// assert
		assert.ErrorIs(t, err, seal.ErrTampered)
	})
}

func TestParseKeyRejects(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{"text that is not base64", "not base64!"},
		{"a key that is too short", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"an empty key", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			_, err := seal.ParseKey(tc.encoded)

			// assert
			assert.Error(t, err)
		})
	}
}

func TestKeyIDsDifferAndRevealNothing(t *testing.T) {
	// arrange
	a, b := mustKey(t, keyA), mustKey(t, keyB)

	// assert
	assert.NotEqual(t, a.ID(), b.ID())
	assert.Len(t, a.ID(), 16)
	assert.NotContains(t, keyA, a.ID())
}

func TestDerivedKeysDependOnPurposeAndActiveKey(t *testing.T) {
	// arrange
	box := seal.NewBox(mustKey(t, keyA))
	rotated := seal.NewBox(mustKey(t, keyB), mustKey(t, keyA))

	// act
	first, err := box.Derive("login cookie hash", 32)
	require.NoError(t, err)
	again, _ := box.Derive("login cookie hash", 32)
	other, _ := box.Derive("login cookie encryption", 32)
	afterRotation, _ := rotated.Derive("login cookie hash", 32)

	// assert
	assert.Len(t, first, 32)
	assert.Equal(t, first, again, "derivation is deterministic")
	assert.NotEqual(t, first, other, "each purpose gets its own key")
	assert.NotEqual(t, first, afterRotation, "a rotation changes every derived key")
}
