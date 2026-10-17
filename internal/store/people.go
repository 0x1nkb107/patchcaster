package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// ErrLastAdministrator reports a change that would leave the instance
// without an administrator.
var ErrLastAdministrator = errors.New("the instance needs at least one administrator")

// Person is someone who logged in through the identity provider, or whom the
// recovery command named (SYSREQ-037, SYSREQ-038).
type Person struct {
	ID            int64
	Issuer        string
	Subject       string
	DisplayName   string
	Email         string
	Administrator bool
	Version       int64
}

// Login is what a successful login tells about the person.
type Login struct {
	Issuer      string
	Subject     string
	DisplayName string
	Email       string
}

// personFrom reads a person row. Every person query returns the same
// columns, so their row types convert to this one.
func personFrom(row sqlcgen.PersonGetRow) *Person {
	return &Person{
		ID:            row.ID,
		Issuer:        row.Issuer,
		Subject:       row.Subject,
		DisplayName:   row.DisplayName,
		Email:         row.Email,
		Administrator: row.IsAdministrator,
		Version:       row.Version,
	}
}

// personID renders a person's id as an audit target.
func personID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// personTarget is how the audit log shows a person.
func personTarget(p *Person) map[string]any {
	return map[string]any{
		"issuer":        p.Issuer,
		"subject":       p.Subject,
		"administrator": p.Administrator,
	}
}

// ActorOf names a person as the actor of a change. The label keeps the
// entry readable after the person's name changes.
func ActorOf(p *Person) Actor {
	label := p.DisplayName
	if label == "" {
		label = p.Subject
	}
	return Actor{PersonID: p.ID, Label: label}
}

// upsertLogin records a login in the transaction q belongs to.
func upsertLogin(ctx context.Context, q *sqlcgen.Queries, login Login, now time.Time) (*Person, error) {
	row, err := q.PersonUpsertLogin(ctx, sqlcgen.PersonUpsertLoginParams{
		Issuer:      login.Issuer,
		Subject:     login.Subject,
		DisplayName: login.DisplayName,
		Email:       login.Email,
		CreatedAt:   now,
	})
	if err != nil {
		return nil, fmt.Errorf("record login of %q: %w", login.Subject, err)
	}
	return personFrom(sqlcgen.PersonGetRow(row)), nil
}

// RecordLogin creates the person at their first login, with the role User,
// and refreshes their name and email at later ones. An administrator's login
// is recorded in the audit log (SYSREQ-041).
func (p *Postgres) RecordLogin(ctx context.Context, login Login, now time.Time) (*Person, error) {
	var person *Person
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		var err error
		person, err = upsertLogin(ctx, q, login, now)
		if err != nil {
			return err
		}
		if !person.Administrator {
			return nil
		}
		return audit(ctx, q, ActorOf(person), now, change{
			action:     "administrator.login",
			targetKind: "person",
			targetID:   personID(person.ID),
		})
	})
	return person, err
}

// RecordRefusedLogin records a login the provider confirmed but PatchCaster
// refused, with the reason (SYSREQ-037).
func (p *Postgres) RecordRefusedLogin(ctx context.Context, login Login, reason string, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		return audit(ctx, q, Actor{Label: login.Subject}, now, change{
			action:     "login.refused",
			targetKind: "login",
			targetID:   login.Subject,
			after: map[string]any{
				"issuer":  login.Issuer,
				"subject": login.Subject,
				"reason":  reason,
			},
		})
	})
}

// Person reads one person, or returns ErrNotFound.
func (p *Postgres) Person(ctx context.Context, id int64) (*Person, error) {
	row, err := p.base.PersonGet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read person %d: %w", id, err)
	}
	return personFrom(row), nil
}

// People pages through people in the order they first logged in, starting
// after the person with afterID.
func (p *Postgres) People(ctx context.Context, afterID int64, limit int) ([]Person, error) {
	rows, err := p.base.PersonList(ctx, sqlcgen.PersonListParams{
		AfterID:  afterID,
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	people := make([]Person, 0, len(rows))
	for _, row := range rows {
		people = append(people, *personFrom(sqlcgen.PersonGetRow(row)))
	}
	return people, nil
}

// AdministratorCount counts the administrators.
func (p *Postgres) AdministratorCount(ctx context.Context) (int, error) {
	n, err := p.base.AdministratorCount(ctx)
	if err != nil {
		return 0, fmt.Errorf("count administrators: %w", err)
	}
	return int(n), nil
}

// SetAdministrator changes a person's role at the version the caller read.
// It refuses to demote the last administrator.
func (p *Postgres) SetAdministrator(ctx context.Context, actor Actor, id int64, administrator bool, version int64, now time.Time) (*Person, error) {
	var updated *Person
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.PersonGet(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read person %d: %w", id, err)
		}
		before := personFrom(row)
		// A stale change is reported as stale, whatever else it would hit.
		if before.Version != version {
			return ErrVersionMismatch
		}
		if before.Administrator && !administrator {
			if err := keepAnAdministrator(ctx, q); err != nil {
				return err
			}
		}
		n, err := q.PersonSetAdministrator(ctx, sqlcgen.PersonSetAdministratorParams{
			ID:              id,
			IsAdministrator: administrator,
			Version:         version,
		})
		if err != nil {
			return fmt.Errorf("change role of person %d: %w", id, err)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		after := *before
		after.Administrator = administrator
		after.Version = version + 1
		updated = &after
		return audit(ctx, q, actor, now, change{
			action:     "person.role",
			targetKind: "person",
			targetID:   personID(id),
			before:     personTarget(before),
			after:      personTarget(&after),
		})
	})
	return updated, err
}

// keepAnAdministrator refuses to demote the only administrator left.
func keepAnAdministrator(ctx context.Context, q *sqlcgen.Queries) error {
	admins, err := q.AdministratorCount(ctx)
	if err != nil {
		return fmt.Errorf("count administrators: %w", err)
	}
	if admins <= 1 {
		return ErrLastAdministrator
	}
	return nil
}

// GrantAdministrator makes the person with issuer and subject an
// administrator, creating the person when they never logged in. It serves
// the recovery command (SYSREQ-038).
func (p *Postgres) GrantAdministrator(ctx context.Context, actor Actor, issuer, subject string, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		id, err := q.PersonGrantAdministrator(ctx, sqlcgen.PersonGrantAdministratorParams{
			Issuer:    issuer,
			Subject:   subject,
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("grant administrator to %q: %w", subject, err)
		}
		return audit(ctx, q, actor, now, change{
			action:     "person.recovery_grant",
			targetKind: "person",
			targetID:   personID(id),
			after: map[string]any{
				"issuer":        issuer,
				"subject":       subject,
				"administrator": true,
			},
		})
	})
}
