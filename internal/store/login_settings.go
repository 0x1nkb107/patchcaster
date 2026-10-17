package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// LoginSettings is the OpenID Connect provider an administrator configured.
// ClientSecret is sealed with the instance key.
type LoginSettings struct {
	Issuer        string
	ClientID      string
	ClientSecret  []byte
	GroupsClaim   string
	AllowedGroups []string
	Version       int64
}

// secretChanged is all the audit log ever says about a secret (SYSREQ-041).
const secretChanged = "secret changed"

// loginSettingsFrom reads the login settings row.
func loginSettingsFrom(row sqlcgen.LoginSettingsGetRow) (*LoginSettings, error) {
	var groups []string
	if err := json.Unmarshal(row.AllowedGroups, &groups); err != nil {
		return nil, fmt.Errorf("decode allowed groups: %w", err)
	}
	return &LoginSettings{
		Issuer:        row.Issuer,
		ClientID:      row.ClientID,
		ClientSecret:  row.ClientSecret,
		GroupsClaim:   row.GroupsClaim,
		AllowedGroups: groups,
		Version:       row.Version,
	}, nil
}

// groupList returns groups as a list that encodes as a JSON array, never as
// null.
func groupList(groups []string) []string {
	if groups == nil {
		return []string{}
	}
	return groups
}

// encodeGroups encodes allowed groups for their JSONB column.
func encodeGroups(groups []string) (json.RawMessage, error) {
	encoded, err := json.Marshal(groupList(groups))
	if err != nil {
		return nil, fmt.Errorf("encode allowed groups: %w", err)
	}
	return encoded, nil
}

// loginSettingsTarget is how the audit log shows login settings. It leaves
// the client secret out. A change of it shows only as "secret changed".
func loginSettingsTarget(s *LoginSettings, newSecret bool) map[string]any {
	target := map[string]any{
		"issuer":         s.Issuer,
		"client_id":      s.ClientID,
		"groups_claim":   s.GroupsClaim,
		"allowed_groups": groupList(s.AllowedGroups),
	}
	if newSecret {
		target["client_secret"] = secretChanged
	}
	return target
}

// readLoginSettings reads the settings in the transaction q belongs to, or
// returns ErrNotFound before the setup stored them.
func readLoginSettings(ctx context.Context, q *sqlcgen.Queries) (*LoginSettings, error) {
	row, err := q.LoginSettingsGet(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read login settings: %w", err)
	}
	return loginSettingsFrom(row)
}

// LoginSettings reads the configured provider, or returns ErrNotFound before
// the setup stored one.
func (p *Postgres) LoginSettings(ctx context.Context) (*LoginSettings, error) {
	return readLoginSettings(ctx, p.base)
}

// UpdateLoginSettings changes the provider at the version in s, the one the
// caller read. A nil ClientSecret keeps the stored secret. The settings it
// returns carry no client secret.
func (p *Postgres) UpdateLoginSettings(ctx context.Context, actor Actor, s LoginSettings, now time.Time) (*LoginSettings, error) {
	groups, err := encodeGroups(s.AllowedGroups)
	if err != nil {
		return nil, err
	}
	var updated *LoginSettings
	err = p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readLoginSettings(ctx, q)
		if err != nil {
			return err
		}
		n, err := q.LoginSettingsUpdate(ctx, sqlcgen.LoginSettingsUpdateParams{
			Issuer:        s.Issuer,
			ClientID:      s.ClientID,
			ClientSecret:  s.ClientSecret,
			GroupsClaim:   s.GroupsClaim,
			AllowedGroups: groups,
			UpdatedAt:     now,
			Version:       s.Version,
		})
		if err != nil {
			return fmt.Errorf("update login settings: %w", err)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		after := s
		after.ClientSecret = nil
		after.Version = s.Version + 1
		updated = &after
		return audit(ctx, q, actor, now, change{
			action:     "login_settings.update",
			targetKind: "login_settings",
			before:     loginSettingsTarget(before, false),
			after:      loginSettingsTarget(&after, s.ClientSecret != nil),
		})
	})
	return updated, err
}

// ReplaceLoginClientSecret stores the client secret sealed again, as long as
// the stored one is still sealed. It reports false when an administrator
// replaced the secret meanwhile, which sealed it with the active key anyway.
// A reseal changes no setting, so it is not audited.
func (p *Postgres) ReplaceLoginClientSecret(ctx context.Context, sealed, resealed []byte) (bool, error) {
	n, err := p.base.LoginClientSecretReplace(ctx, sqlcgen.LoginClientSecretReplaceParams{
		Resealed: resealed,
		Sealed:   sealed,
	})
	if err != nil {
		return false, fmt.Errorf("replace client secret: %w", err)
	}
	return n == 1, nil
}
