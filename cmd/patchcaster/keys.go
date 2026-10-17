package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
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

// resealStore is the persistence a reseal needs: the key check and every
// stored secret.
type resealStore interface {
	keyCheckStore
	LoginSettings(ctx context.Context) (*store.LoginSettings, error)
	ReplaceLoginClientSecret(ctx context.Context, sealed, resealed []byte) (bool, error)
	ConnectionCredentials(ctx context.Context) ([]store.SealedValue, error)
	ReplaceConnectionCredential(ctx context.Context, id int64, sealed, resealed []byte) (bool, error)
	SealedSigningTokens(ctx context.Context) ([]store.SealedValue, error)
	ReplaceSigningToken(ctx context.Context, id int64, sealed, resealed []byte) (bool, error)
}

// resealSecrets seals every stored secret again with the active key and
// returns how many it moved. Secrets already sealed by the active key stay.
// The key check goes first, so that a reseal never runs with a wrong key.
func resealSecrets(ctx context.Context, st resealStore, box *seal.Box, now time.Time) (int, error) {
	moved, err := resealKeyCheck(ctx, st, box, now)
	if err != nil {
		return 0, err
	}
	clientSecret, err := resealClientSecret(ctx, st, box)
	moved += clientSecret
	if err != nil {
		return moved, err
	}
	for _, rows := range sealedTables(st) {
		resealed, err := resealRows(ctx, rows, box)
		moved += resealed
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// resealKeyCheck seals the key check again when a previous key sealed it. It
// refuses keys that do not open it.
func resealKeyCheck(ctx context.Context, st keyCheckStore, box *seal.Box, now time.Time) (int, error) {
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

// resealClientSecret seals the identity provider's client secret again when
// a previous key sealed it. Before the setup there is none.
func resealClientSecret(ctx context.Context, st resealStore, box *seal.Box) (int, error) {
	settings, err := st.LoginSettings(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	resealed, stale, err := resealValue(box, settings.ClientSecret, auth.ClientSecretBinding)
	if err != nil || !stale {
		return 0, err
	}
	replaced, err := st.ReplaceLoginClientSecret(ctx, settings.ClientSecret, resealed)
	if err != nil || !replaced {
		return 0, err
	}
	return 1, nil
}

// sealedRows is one table of stored secrets as a reseal walks it: how to
// list them, what each is sealed for, and how to replace one that is still
// as listed.
type sealedRows struct {
	list    func(ctx context.Context) ([]store.SealedValue, error)
	binding func(id int64) string
	replace func(ctx context.Context, id int64, sealed, resealed []byte) (bool, error)
}

// sealedTables lists the tables that hold a sealed secret per row.
func sealedTables(st resealStore) []sealedRows {
	return []sealedRows{
		{
			list:    st.ConnectionCredentials,
			binding: store.ConnectionCredentialBinding,
			replace: st.ReplaceConnectionCredential,
		},
		{
			list:    st.SealedSigningTokens,
			binding: store.SigningTokenBinding,
			replace: st.ReplaceSigningToken,
		},
	}
}

// resealRows seals the stale secrets of one table again and returns how
// many it moved. A secret replaced meanwhile was sealed with the active key
// anyway, so it is left as it is.
func resealRows(ctx context.Context, rows sealedRows, box *seal.Box) (int, error) {
	values, err := rows.list(ctx)
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, value := range values {
		resealed, stale, err := resealValue(box, value.Sealed, rows.binding(value.ID))
		if err != nil {
			return moved, err
		}
		if !stale {
			continue
		}
		replaced, err := rows.replace(ctx, value.ID, value.Sealed, resealed)
		if err != nil {
			return moved, err
		}
		if replaced {
			moved++
		}
	}
	return moved, nil
}

// resealValue seals one stored secret again with the active key. It reports
// false, and returns nothing, when the active key sealed it already.
func resealValue(box *seal.Box, sealed []byte, binding string) ([]byte, bool, error) {
	stale, err := box.Stale(sealed)
	if err != nil || !stale {
		return nil, false, err
	}
	plaintext, err := box.Open(sealed, binding)
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", binding, err)
	}
	resealed, err := box.Seal(plaintext, binding)
	if err != nil {
		return nil, false, fmt.Errorf("seal %s: %w", binding, err)
	}
	return resealed, true, nil
}
