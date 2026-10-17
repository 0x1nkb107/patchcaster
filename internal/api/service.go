package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Store is what the v1 operations need from persistence.
type Store interface {
	AdministratorCount(ctx context.Context) (int, error)
	Person(ctx context.Context, id int64) (*store.Person, error)
	People(ctx context.Context, afterID int64, limit int) ([]store.Person, error)
	SetAdministrator(ctx context.Context, actor store.Actor, id int64, administrator bool, version int64, now time.Time) (*store.Person, error)
	LoginSettings(ctx context.Context) (*store.LoginSettings, error)
	UpdateLoginSettings(ctx context.Context, actor store.Actor, s store.LoginSettings, now time.Time) (*store.LoginSettings, error)
	SetupLinkValid(ctx context.Context, linkHash []byte, now time.Time) (bool, error)
	PutSetupLoginSettings(ctx context.Context, actor store.Actor, linkHash []byte, s store.LoginSettings, now time.Time) error
	AuditEntries(ctx context.Context, beforeID int64, limit int) ([]store.AuditEntry, error)
}

// Login is what the v1 operations need from the login flow.
type Login interface {
	Check(ctx context.Context, issuer string) error
	URL() string
}

// Config wires the v1 operations. Now defaults to the wall clock.
type Config struct {
	Version  string
	Store    Store
	Box      *seal.Box
	Sessions *scs.SessionManager
	Login    Login
	Log      *slog.Logger
	Now      func() time.Time
}

// Service implements the v1 operations.
type Service struct {
	version  string
	store    Store
	box      *seal.Box
	sessions *scs.SessionManager
	login    Login
	log      *slog.Logger
	now      func() time.Time
}

var _ v1gen.Handler = (*Service)(nil)

// NewService returns the v1 operations.
func NewService(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		version:  cfg.Version,
		store:    cfg.Store,
		box:      cfg.Box,
		sessions: cfg.Sessions,
		login:    cfg.Login,
		log:      cfg.Log,
		now:      now,
	}
}

// NewError turns an error a handler or the security check returned into a
// problem. Anything that is not a client's mistake is logged and reported as
// an internal error without its message, which may reveal internals.
func (s *Service) NewError(ctx context.Context, err error) *v1gen.ProblemStatusCode {
	problem := problemFor(err)
	if problem.StatusCode >= http.StatusInternalServerError {
		s.log.ErrorContext(ctx, "management API request failed", "err", err)
	}
	return problem
}

// personKey carries the authenticated person through a request's context.
type personKey struct{}

// errNoSession reports a session cookie that names no live session of a
// person who still exists.
var errNoSession = errors.New("no valid session")

// security resolves the session cookie of every operation that needs one.
type security struct{ svc *Service }

// HandleSession finds the person the request's session belongs to. The
// session manager's middleware has already loaded the session from the same
// cookie.
func (s security) HandleSession(ctx context.Context, _ v1gen.OperationName, _ v1gen.Session) (context.Context, error) {
	id := auth.PersonID(ctx, s.svc.sessions)
	if id == 0 {
		return ctx, errNoSession
	}
	person, err := s.svc.store.Person(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return ctx, errNoSession
	}
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, personKey{}, person), nil
}

// caller returns the authenticated person of a request. Every operation
// with session security has one.
func caller(ctx context.Context) *store.Person {
	person, _ := ctx.Value(personKey{}).(*store.Person)
	return person
}

// administrator returns the caller when they are an administrator, and a
// 403 problem otherwise (SYSREQ-039).
func administrator(ctx context.Context) (*store.Person, error) {
	person := caller(ctx)
	if person == nil || !person.Administrator {
		return nil, newProblem(http.StatusForbidden, "only administrators may do this")
	}
	return person, nil
}
