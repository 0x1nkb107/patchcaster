package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/seal"
)

// KeyFlags carry the instance encryption key that seals every stored secret
// (ADR-035). Prefer the environment variables, because flags show up in
// process listings.
type KeyFlags struct {
	EncryptionKey          string   `flag:"" required:"" env:"PATCHCASTER_ENCRYPTION_KEY" help:"Instance encryption key: 32 random bytes, base64-encoded."`
	PreviousEncryptionKeys []string `flag:"" name:"previous-encryption-key" sep:"," env:"PATCHCASTER_PREVIOUS_ENCRYPTION_KEYS" help:"Earlier encryption key that still opens stored secrets during a rotation; repeat or comma-separate."`
}

// validate rejects malformed keys. An absent active key passes, because kong
// reports the missing required flag itself, and only after the Validate hooks
// have run.
func (k KeyFlags) validate() error {
	if k.EncryptionKey == "" {
		return nil
	}
	_, err := k.box()
	return err
}

// box parses the keys into a box that seals with the active key and opens
// with any of them. Its errors name the flag, never the key.
func (k KeyFlags) box() (*seal.Box, error) {
	active, err := seal.ParseKey(k.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("--encryption-key: %w", err)
	}
	previous := make([]seal.Key, 0, len(k.PreviousEncryptionKeys))
	for i, encoded := range k.PreviousEncryptionKeys {
		key, err := seal.ParseKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("--previous-encryption-key %d: %w", i+1, err)
		}
		previous = append(previous, key)
	}
	return seal.NewBox(active, previous...), nil
}

// The key check is a known value sealed with the instance key. Opening it
// proves that the configured keys open this database's secrets (SYSREQ-042).
const (
	keyCheckPlaintext = "patchcaster instance key check"
	keyCheckBinding   = "instance_key_check.sealed.1"
)

// errWrongKey names the problem and the remedy without naming any key.
var errWrongKey = errors.New("the encryption key does not open this database's secrets: " +
	"start with the key that sealed them, or pass it as a previous key")

// keyCheckStore is the persistence the key check needs.
type keyCheckStore interface {
	KeyCheck(ctx context.Context) ([]byte, error)
	InitKeyCheck(ctx context.Context, sealed []byte, now time.Time) error
	ReplaceKeyCheck(ctx context.Context, sealed []byte, now time.Time) error
}

// verifyInstanceKey makes sure the configured keys open the database's
// secrets. The first start stores the key check. It reports whether the
// secrets are still sealed by a previous key and so need a reseal.
func verifyInstanceKey(ctx context.Context, st keyCheckStore, box *seal.Box, now time.Time) (bool, error) {
	sealed, err := st.KeyCheck(ctx)
	if err != nil {
		return false, err
	}
	if sealed == nil {
		sealed, err = initKeyCheck(ctx, st, box, now)
		if err != nil {
			return false, err
		}
	}
	if err := openKeyCheck(box, sealed); err != nil {
		return false, err
	}
	return box.Stale(sealed)
}

// initKeyCheck stores a first key check and returns the one that won. When
// another instance stored one at the same moment, that one is kept and
// verified instead.
func initKeyCheck(ctx context.Context, st keyCheckStore, box *seal.Box, now time.Time) ([]byte, error) {
	fresh, err := box.Seal([]byte(keyCheckPlaintext), keyCheckBinding)
	if err != nil {
		return nil, fmt.Errorf("seal key check: %w", err)
	}
	if err := st.InitKeyCheck(ctx, fresh, now); err != nil {
		return nil, err
	}
	return st.KeyCheck(ctx)
}

// openKeyCheck turns any failure to open the key check into errWrongKey.
func openKeyCheck(box *seal.Box, sealed []byte) error {
	plaintext, err := box.Open(sealed, keyCheckBinding)
	if errors.Is(err, seal.ErrUnknownKey) || errors.Is(err, seal.ErrTampered) {
		return errWrongKey
	}
	if err != nil {
		return fmt.Errorf("open key check: %w", err)
	}
	if string(plaintext) != keyCheckPlaintext {
		return errWrongKey
	}
	return nil
}

// resealSecrets seals every stored secret again with the active key and
// returns how many it moved. Secrets already sealed by the active key stay.
func resealSecrets(ctx context.Context, st keyCheckStore, box *seal.Box, now time.Time) (int, error) {
	sealed, err := st.KeyCheck(ctx)
	if err != nil {
		return 0, err
	}
	if sealed == nil {
		return 0, nil
	}
	if err := openKeyCheck(box, sealed); err != nil {
		return 0, err
	}
	stale, err := box.Stale(sealed)
	if err != nil || !stale {
		return 0, err
	}
	fresh, err := box.Seal([]byte(keyCheckPlaintext), keyCheckBinding)
	if err != nil {
		return 0, fmt.Errorf("seal key check: %w", err)
	}
	if err := st.ReplaceKeyCheck(ctx, fresh, now); err != nil {
		return 0, err
	}
	return 1, nil
}
