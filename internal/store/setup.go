package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

var (
	// ErrSetupLinkSpent reports a setup link that was used, has expired or
	// never existed (SYSREQ-038).
	ErrSetupLinkSpent = errors.New("the setup link is used, expired or unknown")
	// ErrAdministratorExists reports a setup step after the setup is
	// complete: once an administrator exists, the setup is closed.
	ErrAdministratorExists = errors.New("an administrator exists")
)

// setupClosed returns ErrAdministratorExists once an administrator exists.
func setupClosed(ctx context.Context, q *sqlcgen.Queries) error {
	admins, err := q.AdministratorCount(ctx)
	if err != nil {
		return fmt.Errorf("count administrators: %w", err)
	}
	if admins > 0 {
		return ErrAdministratorExists
	}
	return nil
}

// setupLinkValid reports whether a setup link is unused and unexpired, in
// the transaction q belongs to.
func setupLinkValid(ctx context.Context, q *sqlcgen.Queries, linkHash []byte, now time.Time) (bool, error) {
	valid, err := q.SetupLinkValid(ctx, sqlcgen.SetupLinkValidParams{
		TokenHash: linkHash,
		ExpiresAt: now,
	})
	if err != nil {
		return false, fmt.Errorf("check setup link: %w", err)
	}
	return valid, nil
}

// CreateSetupLink stores the hash of a new setup link that expires at
// expires. It refuses once an administrator exists (SYSREQ-038).
func (p *Postgres) CreateSetupLink(ctx context.Context, actor Actor, linkHash []byte, now, expires time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		if err := setupClosed(ctx, q); err != nil {
			return err
		}
		err := q.SetupLinkCreate(ctx, sqlcgen.SetupLinkCreateParams{
			TokenHash: linkHash,
			CreatedAt: now,
			ExpiresAt: expires,
		})
		if err != nil {
			return fmt.Errorf("store setup link: %w", err)
		}
		return audit(ctx, q, actor, now, change{
			action:     "setup.link_issued",
			targetKind: "setup_link",
			after:      map[string]any{"expires_at": expires},
		})
	})
}

// SetupLinkValid reports whether a setup link is unused and unexpired.
func (p *Postgres) SetupLinkValid(ctx context.Context, linkHash []byte, now time.Time) (bool, error) {
	return setupLinkValid(ctx, p.base, linkHash, now)
}

// PutSetupLoginSettings stores the provider a setup names, replacing what an
// earlier attempt of the same setup stored. The setup link must still be
// valid and no administrator may exist yet, so that nobody without the link
// can choose the provider.
func (p *Postgres) PutSetupLoginSettings(ctx context.Context, actor Actor, linkHash []byte, s LoginSettings, now time.Time) error {
	groups, err := encodeGroups(s.AllowedGroups)
	if err != nil {
		return err
	}
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		if err := setupClosed(ctx, q); err != nil {
			return err
		}
		valid, err := setupLinkValid(ctx, q, linkHash, now)
		if err != nil {
			return err
		}
		if !valid {
			return ErrSetupLinkSpent
		}
		var before any
		earlier, err := readLoginSettings(ctx, q)
		switch {
		case err == nil:
			before = loginSettingsTarget(earlier, false)
		case !errors.Is(err, ErrNotFound):
			return err
		}
		err = q.LoginSettingsPut(ctx, sqlcgen.LoginSettingsPutParams{
			Issuer:        s.Issuer,
			ClientID:      s.ClientID,
			ClientSecret:  s.ClientSecret,
			GroupsClaim:   s.GroupsClaim,
			AllowedGroups: groups,
			UpdatedAt:     now,
		})
		if err != nil {
			return fmt.Errorf("store login settings: %w", err)
		}
		return audit(ctx, q, actor, now, change{
			action:     "login_settings.setup",
			targetKind: "login_settings",
			before:     before,
			after:      loginSettingsTarget(&s, true),
		})
	})
}

// CompleteSetup spends the setup link and makes the person who logged in
// through it the first administrator, all in one transaction (SYSREQ-038).
func (p *Postgres) CompleteSetup(ctx context.Context, linkHash []byte, login Login, now time.Time) (*Person, error) {
	var person *Person
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		used, err := q.SetupLinkUse(ctx, sqlcgen.SetupLinkUseParams{
			TokenHash: linkHash,
			UsedAt:    sql.NullTime{Time: now, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("spend setup link: %w", err)
		}
		if used == 0 {
			return ErrSetupLinkSpent
		}
		if err := setupClosed(ctx, q); err != nil {
			return err
		}
		person, err = upsertLogin(ctx, q, login, now)
		if err != nil {
			return err
		}
		_, err = q.PersonGrantAdministrator(ctx, sqlcgen.PersonGrantAdministratorParams{
			Issuer:    login.Issuer,
			Subject:   login.Subject,
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("make %q the first administrator: %w", login.Subject, err)
		}
		// The grant changed the row the login created, so its version moved on.
		person.Administrator = true
		person.Version++
		return audit(ctx, q, ActorOf(person), now, change{
			action:     "setup.complete",
			targetKind: "person",
			targetID:   personID(person.ID),
			after:      personTarget(person),
		})
	})
	return person, err
}
