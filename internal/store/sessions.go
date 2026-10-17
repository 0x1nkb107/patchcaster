package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// SessionStore keeps login sessions in PostgreSQL in the shape the scs
// session manager expects (ADR-033). It implements scs's CtxStore without
// importing scs, so no session library type crosses the store boundary.
type SessionStore struct {
	base *sqlcgen.Queries
	now  func() time.Time
}

// Sessions returns the session store over this database.
func (p *Postgres) Sessions() *SessionStore {
	return &SessionStore{base: p.base, now: time.Now}
}

// FindCtx returns the data of an unexpired session.
func (s *SessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	data, err := s.base.SessionFind(ctx, sqlcgen.SessionFindParams{Token: token, Expiry: s.now()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find session: %w", err)
	}
	return data, true, nil
}

// CommitCtx stores a session until expiry.
func (s *SessionStore) CommitCtx(ctx context.Context, token string, data []byte, expiry time.Time) error {
	if err := s.base.SessionCommit(ctx, sqlcgen.SessionCommitParams{Token: token, Data: data, Expiry: expiry}); err != nil {
		return fmt.Errorf("store session: %w", err)
	}
	return nil
}

// DeleteCtx ends a session. Ending an unknown session is no error.
func (s *SessionStore) DeleteCtx(ctx context.Context, token string) error {
	if err := s.base.SessionDelete(ctx, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// Find, Commit and Delete serve callers without a context.

func (s *SessionStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s *SessionStore) Commit(token string, data []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, data, expiry)
}

func (s *SessionStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

// DeleteExpiredSessions removes sessions past their expiry.
func (p *Postgres) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	if err := p.base.SessionDeleteExpired(ctx, now); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
