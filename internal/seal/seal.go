// Package seal encrypts the secrets PatchCaster stores with the instance key
// (ADR-035). A sealed value is bound to the place it is stored in, so a value
// copied into another row or column fails to open.
package seal

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// KeySize is the length of an instance key: AES-256.
const KeySize = 32

// A sealed value is laid out as version, key id, nonce, then the AES-GCM
// ciphertext with its tag. The key id names the key that sealed the value, so
// a rotation can open old values with a previous key.
const (
	formatVersion byte = 1
	keyIDSize          = 8
	nonceSize          = 12
	headerSize         = 1 + keyIDSize + nonceSize
)

var (
	// ErrMalformed reports a sealed value whose layout PatchCaster does not
	// recognize.
	ErrMalformed = errors.New("sealed value is malformed")
	// ErrUnknownKey reports a sealed value that none of the configured keys
	// sealed.
	ErrUnknownKey = errors.New("sealed value was sealed by a key that is not configured")
	// ErrTampered reports a sealed value that fails authentication: it was
	// changed, or it is opened for another place than it was sealed for.
	ErrTampered = errors.New("sealed value fails authentication")
)

// Key is one instance encryption key.
type Key struct {
	id   [keyIDSize]byte
	aead cipher.AEAD
}

// ParseKey decodes a base64-encoded key of KeySize bytes.
func ParseKey(encoded string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return Key{}, errors.New("key is not valid base64")
	}
	if len(raw) != KeySize {
		return Key{}, fmt.Errorf("key decodes to %d bytes, want %d", len(raw), KeySize)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return Key{}, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return Key{}, fmt.Errorf("create GCM: %w", err)
	}
	var key Key
	digest := sha256.Sum256(raw)
	copy(key.id[:], digest[:keyIDSize])
	key.aead = aead
	return key, nil
}

// ID identifies the key in logs without revealing it: the hex-encoded start
// of its SHA-256 digest.
func (k Key) ID() string {
	return hex.EncodeToString(k.id[:])
}

// Box seals with the active key and opens with the active key or any
// previous one, which is how a key rotates without downtime.
type Box struct {
	active   Key
	previous []Key
}

// NewBox returns a box that seals with active and also opens values sealed by
// any of previous.
func NewBox(active Key, previous ...Key) *Box {
	return &Box{active: active, previous: previous}
}

// Seal encrypts plaintext for the place named by binding, such as
// "connections.credentials.42". Opening it needs the same binding.
func (b *Box) Seal(plaintext []byte, binding string) ([]byte, error) {
	sealed := make([]byte, headerSize, headerSize+len(plaintext)+b.active.aead.Overhead())
	sealed[0] = formatVersion
	copy(sealed[1:], b.active.id[:])
	nonce := sealed[1+keyIDSize : headerSize]
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	return b.active.aead.Seal(sealed, nonce, plaintext, []byte(binding)), nil
}

// Open decrypts a sealed value for the place named by binding.
func (b *Box) Open(sealed []byte, binding string) ([]byte, error) {
	key, err := b.keyFor(sealed)
	if err != nil {
		return nil, err
	}
	nonce := sealed[1+keyIDSize : headerSize]
	plaintext, err := key.aead.Open(nil, nonce, sealed[headerSize:], []byte(binding))
	if err != nil {
		return nil, ErrTampered
	}
	return plaintext, nil
}

// Stale reports whether a sealed value was sealed by a previous key, so that
// a reseal should move it to the active key.
func (b *Box) Stale(sealed []byte) (bool, error) {
	key, err := b.keyFor(sealed)
	if err != nil {
		return false, err
	}
	return key.id != b.active.id, nil
}

// keyFor finds the configured key that sealed a value.
func (b *Box) keyFor(sealed []byte) (Key, error) {
	if len(sealed) < headerSize || sealed[0] != formatVersion {
		return Key{}, ErrMalformed
	}
	id := sealed[1 : 1+keyIDSize]
	if bytes.Equal(id, b.active.id[:]) {
		return b.active, nil
	}
	for _, key := range b.previous {
		if bytes.Equal(id, key.id[:]) {
			return key, nil
		}
	}
	return Key{}, ErrUnknownKey
}
