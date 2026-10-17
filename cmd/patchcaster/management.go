package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/patchcaster-app/patchcaster/internal/api"
	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// managementHandler serves the management listener: the API and the login,
// both inside the session middleware and behind cross-origin protection
// (SYSREQ-036, SYSREQ-037).
func managementHandler(svc *api.Service, login *auth.Login, sessions *scs.SessionManager, interfaceURL *url.URL) (http.Handler, error) {
	mux := http.NewServeMux()
	if err := api.Routes(mux, svc); err != nil {
		return nil, err
	}
	login.Routes(mux)

	// The protection compares a browser's Origin with the Host header. A
	// proxy in front of the interface may rewrite that header, so the
	// interface's own origin is trusted explicitly.
	protection := http.NewCrossOriginProtection()
	if err := protection.AddTrustedOrigin(interfaceURL.Scheme + "://" + interfaceURL.Host); err != nil {
		return nil, fmt.Errorf("trust interface origin: %w", err)
	}
	protection.SetDenyHandler(api.ProblemHandler(http.StatusForbidden,
		"a request from another site may not change anything"))
	return protection.Handler(sessions.LoadAndSave(mux)), nil
}

// expireSessionsLoop removes sessions past their expiry. No request finds
// them anymore, but without the cleanup the table would only grow.
func expireSessionsLoop(ctx context.Context, st *store.Postgres, log *slog.Logger) error {
	expire := func() {
		if err := st.DeleteExpiredSessions(ctx, time.Now()); err != nil {
			log.Error("delete expired sessions failed", "err", err)
		}
	}
	expire()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			expire()
		}
	}
}
