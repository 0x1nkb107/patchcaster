package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Store is what the v1 operations need from persistence.
type Store interface {
	SourceScopes(ctx context.Context, connectionID int64) ([]store.SourceScope, error)
	ForgeHooks(ctx context.Context, connectionID int64) ([]store.ForgeHook, error)
	SinkHealth(ctx context.Context, ids []int64) (map[int64]store.SinkHealth, error)
	ForgeHealth(ctx context.Context, ids []int64) (map[int64]store.ForgeHealth, error)
	ChatConnectionHealth(ctx context.Context, ids []int64) (map[int64]store.SinkHealth, error)
	AdministratorCount(ctx context.Context) (int, error)
	Person(ctx context.Context, id int64) (*store.Person, error)
	People(ctx context.Context, afterID int64, limit int) ([]store.Person, error)
	SetAdministrator(ctx context.Context, actor store.Actor, id int64, administrator bool, version int64, now time.Time) (*store.Person, error)
	LoginSettings(ctx context.Context) (*store.LoginSettings, error)
	UpdateLoginSettings(ctx context.Context, actor store.Actor, s store.LoginSettings, now time.Time) (*store.LoginSettings, error)
	SetupLinkValid(ctx context.Context, linkHash []byte, now time.Time) (bool, error)
	PutSetupLoginSettings(ctx context.Context, actor store.Actor, linkHash []byte, s store.LoginSettings, now time.Time) error
	AuditEntries(ctx context.Context, beforeID int64, limit int) ([]store.AuditEntry, error)
	CreateToken(ctx context.Context, actor store.Actor, t store.NewAPIToken, now time.Time) (*store.APIToken, error)
	AuthenticateToken(ctx context.Context, hash []byte, now time.Time) (*store.APIToken, error)
	Tokens(ctx context.Context, personID, afterID int64, limit int) ([]store.APIToken, error)
	RevokeToken(ctx context.Context, actor store.Actor, personID, tokenID int64, now time.Time) error
	RevokeAccess(ctx context.Context, actor store.Actor, personID int64, now time.Time) (int, error)
	ReserveConnectionID(ctx context.Context) (int64, error)
	CreateConnection(ctx context.Context, actor store.Actor, c store.Connection, now time.Time) (*store.Connection, error)
	Connection(ctx context.Context, id int64) (*store.Connection, error)
	Connections(ctx context.Context, afterID int64, limit int) ([]store.Connection, error)
	UpdateConnection(ctx context.Context, actor store.Actor, c store.Connection, now time.Time) (*store.Connection, error)
	DeleteConnection(ctx context.Context, actor store.Actor, id, version int64, now time.Time) error
	ReserveSigningTokenID(ctx context.Context) (int64, error)
	AddSigningToken(ctx context.Context, actor store.Actor, t store.NewSigningToken, now time.Time) (*store.SigningToken, error)
	SigningTokens(ctx context.Context, connectionID, afterID int64, limit int) ([]store.SigningToken, error)
	RemoveSigningToken(ctx context.Context, actor store.Actor, connectionID, tokenID int64, now time.Time) error
	CreateTeam(ctx context.Context, actor store.Actor, name string, now time.Time) (*store.Team, error)
	Team(ctx context.Context, id int64) (*store.Team, error)
	Teams(ctx context.Context, visibleTo, afterID int64, limit int) ([]store.Team, error)
	UpdateTeam(ctx context.Context, actor store.Actor, t store.Team, now time.Time) (*store.Team, error)
	DeleteTeam(ctx context.Context, actor store.Actor, id, version int64, now time.Time) error
	ManagesTeam(ctx context.Context, personID, teamID int64) (bool, error)
	CreateSource(ctx context.Context, actor store.Actor, s store.Source, now time.Time) (*store.Source, error)
	Source(ctx context.Context, id int64) (*store.Source, error)
	Sources(ctx context.Context, visibleTo, afterID int64, limit int) ([]store.Source, error)
	UpdateSource(ctx context.Context, actor store.Actor, s store.Source, now time.Time) (*store.Source, error)
	DeleteSource(ctx context.Context, actor store.Actor, id, version int64, now time.Time) error
	CreateSink(ctx context.Context, actor store.Actor, s store.Sink, now time.Time) (*store.Sink, error)
	Sink(ctx context.Context, id int64) (*store.Sink, error)
	Sinks(ctx context.Context, visibleTo, afterID int64, limit int) ([]store.Sink, error)
	UpdateSink(ctx context.Context, actor store.Actor, s store.Sink, now time.Time) (*store.Sink, error)
	DeleteSink(ctx context.Context, actor store.Actor, id, version int64, now time.Time) error
	CreateRoute(ctx context.Context, actor store.Actor, r store.Route, now time.Time) (*store.Route, error)
	Route(ctx context.Context, id int64) (*store.Route, error)
	Routes(ctx context.Context, visibleTo, teamID, afterID int64, limit int) ([]store.Route, error)
	UpdateRoute(ctx context.Context, actor store.Actor, r store.Route, now time.Time) (*store.Route, error)
	DeleteRoute(ctx context.Context, actor store.Actor, id, version int64, now time.Time) error
}

// Login is what the v1 operations need from the login flow.
type Login interface {
	Check(ctx context.Context, issuer string) error
	URL() string
}

// Platforms checks connection credentials and sink channels against their
// platforms.
type Platforms interface {
	Check(ctx context.Context, kind platform.Kind, baseURL *url.URL, credential string) (string, error)
	CheckChannel(ctx context.Context, kind platform.Kind, baseURL *url.URL, credential, channelID string) (string, error)
}

// Config wires the v1 operations. PublicURL is the webhook listener's
// external address. Now defaults to the wall clock.
type Config struct {
	Version   string
	Store     Store
	Box       *seal.Box
	Sessions  *scs.SessionManager
	Login     Login
	Platforms Platforms
	Hooks     Hooks
	PublicURL *url.URL
	Log       *slog.Logger
	Now       func() time.Time
}

// Service implements the v1 operations.
type Service struct {
	version   string
	store     Store
	box       *seal.Box
	sessions  *scs.SessionManager
	login     Login
	platforms Platforms
	hooks     Hooks
	publicURL *url.URL
	log       *slog.Logger
	now       func() time.Time
}

var _ v1gen.Handler = (*Service)(nil)

// NewService returns the v1 operations.
func NewService(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		version:   cfg.Version,
		store:     cfg.Store,
		box:       cfg.Box,
		sessions:  cfg.Sessions,
		login:     cfg.Login,
		platforms: cfg.Platforms,
		hooks:     cfg.Hooks,
		publicURL: cfg.PublicURL,
		log:       cfg.Log,
		now:       now,
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

// principal is who a request acts for: a person, and the API token they
// sent, if any. The person is read afresh for every request, so a changed
// role applies at once to sessions and tokens alike (SYSREQ-040).
type principal struct {
	person *store.Person
	token  *store.APIToken
}

// principalKey carries the principal through a request's context.
type principalKey struct{}

var (
	// errNoSession reports a session cookie that names no live session of
	// a person who still exists.
	errNoSession = errors.New("no valid session")
	// errNoToken reports a bearer token that is unknown, revoked or
	// expired.
	errNoToken = errors.New("no valid API token")
)

// security resolves the credentials of every operation that needs them: a
// session cookie or an API token.
type security struct{ svc *Service }

// HandleSession finds the person the request's session belongs to. The
// session manager's middleware has already loaded the session from the same
// cookie. A session from before the person's access was revoked counts for
// nothing.
func (s security) HandleSession(ctx context.Context, _ v1gen.OperationName, _ v1gen.Session) (context.Context, error) {
	id, epoch := auth.SessionOf(ctx, s.svc.sessions)
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
	if person.SessionEpoch != epoch {
		return ctx, errNoSession
	}
	return context.WithValue(ctx, principalKey{}, &principal{person: person}), nil
}

// HandleToken finds the person a live API token belongs to.
func (s security) HandleToken(ctx context.Context, _ v1gen.OperationName, t v1gen.Token) (context.Context, error) {
	token, err := s.svc.store.AuthenticateToken(ctx, auth.TokenHash(t.Token), s.svc.now())
	if errors.Is(err, store.ErrNotFound) {
		return ctx, errNoToken
	}
	if err != nil {
		return ctx, err
	}
	person, err := s.svc.store.Person(ctx, token.PersonID)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, principalKey{}, &principal{person: person, token: token}), nil
}

// caller returns the authenticated person of a request. Every operation
// with security has one.
func caller(ctx context.Context) *store.Person {
	p, _ := ctx.Value(principalKey{}).(*principal)
	if p == nil {
		return nil
	}
	return p.person
}

// actorOf names the caller in the audit log, with the token they used
// (SYSREQ-041).
func actorOf(ctx context.Context) store.Actor {
	p, _ := ctx.Value(principalKey{}).(*principal)
	if p == nil {
		return store.Actor{}
	}
	actor := store.ActorOf(p.person)
	if p.token != nil {
		actor.TokenID = p.token.ID
		actor.TokenName = p.token.Name
	}
	return actor
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
