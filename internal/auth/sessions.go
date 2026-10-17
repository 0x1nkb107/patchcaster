// Package auth logs people in through the organization's OpenID Connect
// provider and keeps their sessions (SYSREQ-037, ADR-033).
package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
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

// StartSession binds a request's session to the person who logged in. It
// renews the session token first, so that a token planted before the login
// is worthless after it (ADR-033).
func StartSession(ctx context.Context, sessions *scs.SessionManager, personID int64) error {
	if err := sessions.RenewToken(ctx); err != nil {
		return fmt.Errorf("renew session token: %w", err)
	}
	sessions.Remove(ctx, sessionSetupLink)
	sessions.Put(ctx, sessionPerson, personID)
	return nil
}

// PersonID returns the person a request's session belongs to, or zero.
func PersonID(ctx context.Context, sessions *scs.SessionManager) int64 {
	return sessions.GetInt64(ctx, sessionPerson)
}

// RememberSetupLink keeps a setup link's hash in the session, so that the
// login that follows can finish the setup.
func RememberSetupLink(ctx context.Context, sessions *scs.SessionManager, hash []byte) {
	sessions.Put(ctx, sessionSetupLink, hash)
}
