package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/zitadel/oidc/v3/pkg/client"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ClientSecretBinding is where the sealed client secret is stored.
const ClientSecretBinding = "login_settings.client_secret.1"

// The scopes PatchCaster asks for: the identity, a name and an email.
var scopes = []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}

// Store is what logging in needs from persistence.
type Store interface {
	LoginSettings(ctx context.Context) (*store.LoginSettings, error)
	AdministratorCount(ctx context.Context) (int, error)
	RecordLogin(ctx context.Context, login store.Login, now time.Time) (*store.Person, error)
	CompleteSetup(ctx context.Context, linkHash []byte, login store.Login, now time.Time) (*store.Person, error)
	RecordRefusedLogin(ctx context.Context, login store.Login, reason string, now time.Time) error
}

// Login runs the authorization code flow with PKCE against the configured
// provider (SYSREQ-037, ADR-033).
type Login struct {
	store        Store
	box          *seal.Box
	sessions     *scs.SessionManager
	interfaceURL *url.URL
	log          *slog.Logger
	now          func() time.Time
	httpClient   *http.Client

	mu     sync.Mutex
	cached *party // the relying party for the stored settings' version
}

// party is a relying party with the settings it was built from.
type party struct {
	version  int64
	settings *store.LoginSettings
	rp       rp.RelyingParty
}

// NewLogin returns the login flow for an instance reachable at interfaceURL.
func NewLogin(st Store, box *seal.Box, sessions *scs.SessionManager, interfaceURL *url.URL, log *slog.Logger) *Login {
	return &Login{
		store:        st,
		box:          box,
		sessions:     sessions,
		interfaceURL: interfaceURL,
		log:          log,
		now:          time.Now,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Routes serves the login and the provider's callback.
func (l *Login) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", l.startLogin)
	mux.HandleFunc("GET /auth/callback", l.finishLogin)
}

// URL returns the address at which a login starts.
func (l *Login) URL() string {
	return l.interfaceURL.JoinPath("auth", "login").String()
}

// Check confirms that an OpenID Connect provider answers at issuer and
// describes itself under that issuer, before an administrator stores it.
// A mistyped issuer is refused instead of locking everyone out. The client
// credentials are only tested by the first login.
func (l *Login) Check(ctx context.Context, issuer string) error {
	if _, err := client.Discover(ctx, issuer, l.httpClient); err != nil {
		return fmt.Errorf("discover identity provider %q: %w", issuer, err)
	}
	return nil
}

// relyingParty builds a relying party, which fetches the provider's
// discovery document.
func (l *Login) relyingParty(ctx context.Context, issuer, clientID, clientSecret string) (rp.RelyingParty, error) {
	cookies, err := l.cookieHandler()
	if err != nil {
		return nil, err
	}
	redirect := l.interfaceURL.JoinPath("auth", "callback").String()
	party, err := rp.NewRelyingPartyOIDC(ctx, issuer, clientID, clientSecret, redirect, scopes,
		rp.WithPKCE(cookies),
		rp.WithHTTPClient(l.httpClient),
		rp.WithErrorHandler(func(w http.ResponseWriter, _ *http.Request, errorType, errorDesc, _ string) {
			l.refuse(w, http.StatusUnauthorized, "the identity provider reported "+errorType+": "+errorDesc)
		}),
		rp.WithUnauthorizedHandler(func(w http.ResponseWriter, _ *http.Request, desc, _ string) {
			l.log.Warn("login failed", "detail", desc)
			l.refuse(w, http.StatusUnauthorized, "the login could not be completed; start it again")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("reach identity provider %q: %w", issuer, err)
	}
	return party, nil
}

// cookieHandler keeps the flow's state and PKCE verifier in encrypted
// cookies. Their keys come from the instance key, so a login that starts
// before a restart still completes after it (ADR-033).
func (l *Login) cookieHandler() (*httphelper.CookieHandler, error) {
	hashKey, err := l.box.Derive("login cookie hash", 32)
	if err != nil {
		return nil, err
	}
	encryptKey, err := l.box.Derive("login cookie encryption", 32)
	if err != nil {
		return nil, err
	}
	opts := []httphelper.CookieHandlerOpt{httphelper.WithSameSite(http.SameSiteLaxMode)}
	if l.interfaceURL.Scheme != "https" {
		opts = append(opts, httphelper.WithUnsecure())
	}
	return httphelper.NewCookieHandler(hashKey, encryptKey, opts...), nil
}

// configured returns the relying party for the stored settings, built again
// whenever an administrator changed them.
func (l *Login) configured(ctx context.Context) (*party, error) {
	settings, err := l.store.LoginSettings(ctx)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cached != nil && l.cached.version == settings.Version {
		return l.cached, nil
	}
	secret, err := l.box.Open(settings.ClientSecret, ClientSecretBinding)
	if err != nil {
		return nil, fmt.Errorf("open client secret: %w", err)
	}
	built, err := l.relyingParty(ctx, settings.Issuer, settings.ClientID, string(secret))
	if err != nil {
		return nil, err
	}
	l.cached = &party{
		version:  settings.Version,
		settings: settings,
		rp:       built,
	}
	return l.cached, nil
}

func (l *Login) startLogin(w http.ResponseWriter, r *http.Request) {
	p, err := l.configured(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		l.refuse(w, http.StatusServiceUnavailable, "no identity provider is configured yet: finish the setup first")
		return
	}
	if err != nil {
		l.log.Error("login unavailable", "err", err)
		l.refuse(w, http.StatusBadGateway, "the identity provider cannot be reached")
		return
	}
	rp.AuthURLHandler(newState, p.rp)(w, r)
}

func (l *Login) finishLogin(w http.ResponseWriter, r *http.Request) {
	p, err := l.configured(r.Context())
	if err != nil {
		l.log.Error("login unavailable", "err", err)
		l.refuse(w, http.StatusBadGateway, "the identity provider cannot be reached")
		return
	}
	rp.CodeExchangeHandler(func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], _ string, _ rp.RelyingParty) {
		l.admit(w, r, p.settings, tokens.IDTokenClaims)
	}, p.rp)(w, r)
}

// admit decides about a login the provider confirmed. During the setup it
// makes the person the first administrator; afterwards it records the login.
func (l *Login) admit(w http.ResponseWriter, r *http.Request, settings *store.LoginSettings, claims *oidc.IDTokenClaims) {
	ctx := r.Context()
	login := store.Login{
		Issuer:      claims.Issuer,
		Subject:     claims.Subject,
		DisplayName: displayName(claims),
		Email:       claims.Email,
	}
	if reason := groupRefusal(settings, claims.Claims); reason != "" {
		l.refuseLogin(w, r, login, reason)
		return
	}
	person, err := l.personFor(ctx, login)
	if errors.Is(err, errSetupIncomplete) || errors.Is(err, store.ErrSetupLinkSpent) {
		l.refuseLogin(w, r, login, "the setup is not complete: open the setup link again")
		return
	}
	if errors.Is(err, store.ErrAdministratorExists) {
		l.refuse(w, http.StatusConflict, "the setup completed meanwhile: log in again")
		return
	}
	if err != nil {
		l.log.Error("record login failed", "err", err)
		l.refuse(w, http.StatusInternalServerError, "the login could not be recorded")
		return
	}
	if err := StartSession(ctx, l.sessions, person.ID); err != nil {
		l.log.Error("start session failed", "err", err)
		l.refuse(w, http.StatusInternalServerError, "the session could not be started")
		return
	}
	http.Redirect(w, r, l.interfaceURL.String()+"/", http.StatusSeeOther)
}

var errSetupIncomplete = errors.New("setup incomplete")

// personFor records the login, or finishes the setup when no administrator
// exists yet and this session opened the setup link.
func (l *Login) personFor(ctx context.Context, login store.Login) (*store.Person, error) {
	admins, err := l.store.AdministratorCount(ctx)
	if err != nil {
		return nil, err
	}
	if admins > 0 {
		return l.store.RecordLogin(ctx, login, l.now())
	}
	linkHash, ok := l.sessions.Get(ctx, sessionSetupLink).([]byte)
	if !ok || len(linkHash) == 0 {
		return nil, errSetupIncomplete
	}
	return l.store.CompleteSetup(ctx, linkHash, login, l.now())
}

// groupRefusal names why the groups in claims do not admit the person, or
// returns "" when they do. Without both a claim and allowed groups, everyone
// the provider authenticates is admitted (ADR-033).
func groupRefusal(settings *store.LoginSettings, claims map[string]any) string {
	if settings.GroupsClaim == "" || len(settings.AllowedGroups) == 0 {
		return ""
	}
	for _, group := range groupsIn(claims[settings.GroupsClaim]) {
		if slices.Contains(settings.AllowedGroups, group) {
			return ""
		}
	}
	return "not a member of a group that may log in"
}

// groupsIn reads a groups claim, which providers send as a list of strings
// or, for a single group, as one string.
func groupsIn(claim any) []string {
	switch value := claim.(type) {
	case string:
		return []string{value}
	case []any:
		groups := make([]string, 0, len(value))
		for _, item := range value {
			if group, ok := item.(string); ok {
				groups = append(groups, group)
			}
		}
		return groups
	}
	return nil
}

func displayName(claims *oidc.IDTokenClaims) string {
	for _, name := range []string{claims.Name, claims.PreferredUsername, claims.Email} {
		if name != "" {
			return name
		}
	}
	return claims.Subject
}

// refuseLogin records a refused login and tells the person why.
func (l *Login) refuseLogin(w http.ResponseWriter, r *http.Request, login store.Login, reason string) {
	if err := l.store.RecordRefusedLogin(r.Context(), login, reason, l.now()); err != nil {
		l.log.Error("record refused login failed", "err", err)
	}
	l.refuse(w, http.StatusForbidden, "PatchCaster refused the login: "+reason)
}

// refuse answers a browser with a short text. The web interface will render
// these pages once it exists.
func (l *Login) refuse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, message)
}

// newState returns a random value that ties the provider's callback to the
// login that started it.
func newState() string {
	return rand.Text()
}
