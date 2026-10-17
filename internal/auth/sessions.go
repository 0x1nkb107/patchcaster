// Package auth logs people in through the organization's OpenID Connect
// provider and keeps their sessions (SYSREQ-037, ADR-033).
package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Session lifetimes of ADR-033: an hour without activity, twelve hours after
// login at the latest.
const (
	sessionIdleTimeout = time.Hour
	sessionLifetime    = 12 * time.Hour
)

// Session keys.
const (
	sessionPerson    = "person_id"
	sessionEpoch     = "session_epoch"
	sessionSetupLink = "setup_link"
)

// SessionCookie names the session cookie. The API description declares the
// same name for its session security scheme.
const SessionCookie = "patchcaster_session"

// NewSessions returns the session manager over store. The cookie is Secure
// when the interface is served over https, so that a plain http interface,
// such as a local one, still works. A store that also takes a context, as
// PatchCaster's own does, gets the request's.
func NewSessions(store scs.Store, secure bool) *scs.SessionManager {
	sessions := scs.New()
	sessions.Store = store
	sessions.IdleTimeout = sessionIdleTimeout
	sessions.Lifetime = sessionLifetime
	sessions.Cookie.Name = SessionCookie
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Secure = secure
	sessions.Cookie.Path = "/"
	return sessions
}

// StartSession binds a request's session to the person who logged in, at
// the person's current session epoch. It renews the session token first, so
// that a token planted before the login is worthless after it (ADR-033).
func StartSession(ctx context.Context, sessions *scs.SessionManager, person *store.Person) error {
	if err := sessions.RenewToken(ctx); err != nil {
		return fmt.Errorf("renew session token: %w", err)
	}
	sessions.Remove(ctx, sessionSetupLink)
	sessions.Put(ctx, sessionPerson, person.ID)
	sessions.Put(ctx, sessionEpoch, person.SessionEpoch)
	return nil
}

// SessionOf returns whom a request's session belongs to: the person's id
// and the session epoch of their login. Both are zero without a login. The
// session counts only while the epoch is still the person's current one.
func SessionOf(ctx context.Context, sessions *scs.SessionManager) (personID, epoch int64) {
	return sessions.GetInt64(ctx, sessionPerson), sessions.GetInt64(ctx, sessionEpoch)
}

// RememberSetupLink keeps a setup link's hash in the session, so that the
// login that follows can finish the setup.
func RememberSetupLink(ctx context.Context, sessions *scs.SessionManager, hash []byte) {
	sessions.Put(ctx, sessionSetupLink, hash)
}
