package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ErrUnknownEndpoint reports a webhook endpoint that no forge connection
// owns.
var ErrUnknownEndpoint = errors.New("no forge connection receives webhooks here")

// KeyStore is what the keyring reads.
type KeyStore interface {
	Connection(ctx context.Context, id int64) (*store.Connection, error)
	SigningTokensOf(ctx context.Context, connectionID int64) ([]store.SealedValue, error)
}

// Keyring opens the signing keys of forge connections (SYSREQ-044). It reads
// them for every delivery, so a signing token an administrator adds or
// removes applies to the next one.
type Keyring struct {
	Store KeyStore
	Box   *seal.Box
}

// SigningKeys returns the HMAC keys of the forge connection with
// connectionID. It returns ErrUnknownEndpoint when no forge connection has
// that id. A connection without signing tokens has no keys, so nothing it
// receives verifies.
func (k Keyring) SigningKeys(ctx context.Context, connectionID int64) ([][]byte, error) {
	connection, err := k.Store.Connection(ctx, connectionID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrUnknownEndpoint
	}
	if err != nil {
		return nil, err
	}
	if !platform.Kind(connection.Kind).IsForge() {
		return nil, ErrUnknownEndpoint
	}
	sealed, err := k.Store.SigningTokensOf(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	keys := make([][]byte, 0, len(sealed))
	for _, value := range sealed {
		token, err := k.Box.Open(value.Sealed, store.SigningTokenBinding(value.ID))
		if err != nil {
			return nil, fmt.Errorf("open signing token %d: %w", value.ID, err)
		}
		key, err := ParseSigningToken(string(token))
		if err != nil {
			return nil, fmt.Errorf("decode signing token %d: %w", value.ID, err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}
