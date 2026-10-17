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

// APIToken is a personal API token as its owner sees it (SYSREQ-040). The
// token itself is never stored, only its hash. RevokedAt is zero while the
// token is not revoked.
type APIToken struct {
	ID        int64
	PersonID  int64
	Name      string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt time.Time
}

// NewAPIToken is a token about to be stored. Hash is the hash of the token
// its owner receives once.
type NewAPIToken struct {
	PersonID  int64
	Name      string
	Hash      []byte
	ExpiresAt time.Time
}

// tokenFrom reads a token row. Every token query returns the same columns,
// so their row types convert to this one.
func tokenFrom(row sqlcgen.TokenGetRow) *APIToken {
	return &APIToken{
		ID:        row.ID,
		PersonID:  row.PersonID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		ExpiresAt: row.ExpiresAt,
		RevokedAt: row.RevokedAt.Time,
	}
}

// tokenTarget is how the audit log shows a token: never with its hash.
func tokenTarget(t *APIToken) map[string]any {
	return map[string]any{
		"person_id":  t.PersonID,
		"name":       t.Name,
		"expires_at": t.ExpiresAt,
	}
}

// CreateToken stores a new token and records it in the audit log.
func (p *Postgres) CreateToken(ctx context.Context, actor Actor, t NewAPIToken, now time.Time) (*APIToken, error) {
	var created *APIToken
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.TokenCreate(ctx, sqlcgen.TokenCreateParams{
			PersonID:  t.PersonID,
			Name:      t.Name,
			TokenHash: t.Hash,
			CreatedAt: now,
			ExpiresAt: t.ExpiresAt,
		})
		if err != nil {
			return fmt.Errorf("store token: %w", err)
		}
		created = tokenFrom(sqlcgen.TokenGetRow(row))
		return audit(ctx, q, actor, now, change{
			action:     "token.create",
			targetKind: "token",
			targetID:   strconv.FormatInt(created.ID, 10),
			after:      tokenTarget(created),
		})
	})
	return created, err
}

// AuthenticateToken finds the token with hash, or returns ErrNotFound when
// no such token exists or it is revoked or expired.
func (p *Postgres) AuthenticateToken(ctx context.Context, hash []byte, now time.Time) (*APIToken, error) {
	row, err := p.base.TokenAuthenticate(ctx, sqlcgen.TokenAuthenticateParams{
		TokenHash: hash,
		ExpiresAt: now,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate token: %w", err)
	}
	return tokenFrom(sqlcgen.TokenGetRow(row)), nil
}

// Tokens pages through a person's tokens in the order they were created,
// starting after the token with afterID. Revoked and expired tokens are
// listed too.
func (p *Postgres) Tokens(ctx context.Context, personID, afterID int64, limit int) ([]APIToken, error) {
	rows, err := p.base.TokenList(ctx, sqlcgen.TokenListParams{
		PersonID: personID,
		AfterID:  afterID,
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	tokens := make([]APIToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, *tokenFrom(sqlcgen.TokenGetRow(row)))
	}
	return tokens, nil
}

// RevokeToken revokes one of a person's tokens. Revoking a revoked token
// changes nothing and records nothing. A token of someone else is
// ErrNotFound, so that ids reveal nothing.
func (p *Postgres) RevokeToken(ctx context.Context, actor Actor, personID, tokenID int64, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.TokenGet(ctx, sqlcgen.TokenGetParams{ID: tokenID, PersonID: personID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read token %d: %w", tokenID, err)
		}
		revoked, err := q.TokenRevoke(ctx, sqlcgen.TokenRevokeParams{
			ID:        tokenID,
			PersonID:  personID,
			RevokedAt: sql.NullTime{Time: now, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("revoke token %d: %w", tokenID, err)
		}
		if revoked == 0 {
			return nil
		}
		return audit(ctx, q, actor, now, change{
			action:     "token.revoke",
			targetKind: "token",
			targetID:   strconv.FormatInt(tokenID, 10),
			before:     tokenTarget(tokenFrom(row)),
		})
	})
}

// RevokeAccess revokes every working token of a person and ends all their
// sessions, in one transaction (SYSREQ-040). It returns how many tokens it
// revoked.
func (p *Postgres) RevokeAccess(ctx context.Context, actor Actor, id int64, now time.Time) (int, error) {
	var revoked int64
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		ended, err := q.PersonEndSessions(ctx, id)
		if err != nil {
			return fmt.Errorf("end sessions of person %d: %w", id, err)
		}
		if ended == 0 {
			return ErrNotFound
		}
		revoked, err = q.TokenRevokeAll(ctx, sqlcgen.TokenRevokeAllParams{
			PersonID:  id,
			RevokedAt: sql.NullTime{Time: now, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("revoke tokens of person %d: %w", id, err)
		}
		return audit(ctx, q, actor, now, change{
			action:     "person.access_revoked",
			targetKind: "person",
			targetID:   personID(id),
			after:      map[string]any{"revoked_tokens": revoked},
		})
	})
	return int(revoked), err
}
