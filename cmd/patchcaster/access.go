package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// parseInterfaceURL reads the web interface's external address, from which
// the login redirect and the setup link are built. It must be an http or
// https origin without a path, because the interface answers at the root of
// its listener.
func parseInterfaceURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("--interface-url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("--interface-url: must start with https:// or http://")
	}
	if u.Host == "" {
		return nil, errors.New("--interface-url: must name a host")
	}
	hasExtras := strings.Trim(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil
	if hasExtras {
		return nil, errors.New("--interface-url: must name only a scheme, a host and a port, because the interface answers at the root of its listener")
	}
	u.Path = ""
	return u, nil
}

// parsePublicURL reads the webhook listener's external address, from which
// the forges' webhook endpoints are built. It must be an http or https URL.
// It may have a path, when a proxy forwards a prefix to the listener.
func parsePublicURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("--public-url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("--public-url: must start with https:// or http://")
	}
	if u.Host == "" {
		return nil, errors.New("--public-url: must name a host")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("--public-url: must name no query, fragment or user")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

// BootstrapCmd issues the single-use setup link that creates the first
// administrator (SYSREQ-038).
type BootstrapCmd struct {
	DatabaseURL  string `flag:"" required:"" env:"PATCHCASTER_DATABASE_URL" help:"PostgreSQL connection URL."`
	InterfaceURL string `flag:"" required:"" env:"PATCHCASTER_INTERFACE_URL" help:"External address of the web interface, such as https://patchcaster.example.org."`
}

// errSetupClosed names the way back in once the setup is complete.
var errSetupClosed = errors.New("an administrator exists, so the setup is closed: " +
	"restore administrator access with patchcaster admin grant")

// Validate rejects a malformed interface URL before anything connects. An
// absent one passes, because kong reports the missing required flag itself.
func (c *BootstrapCmd) Validate() error {
	if c.InterfaceURL == "" {
		return nil
	}
	_, err := parseInterfaceURL(c.InterfaceURL)
	return err
}

// Run stores a new setup link and prints it. The database keeps only its
// hash, so the printed link is the only copy.
func (c *BootstrapCmd) Run(ctx context.Context) error {
	interfaceURL, err := parseInterfaceURL(c.InterfaceURL)
	if err != nil {
		return err
	}
	st, closeDB, err := openMigratedStore(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer closeDB()

	link := auth.NewSetupLink(interfaceURL)
	now := time.Now()
	expires := now.Add(auth.SetupLinkLifetime)
	err = st.CreateSetupLink(ctx, store.Actor{Label: "bootstrap command"}, link.Hash, now, expires)
	if errors.Is(err, store.ErrAdministratorExists) {
		return errSetupClosed
	}
	if err != nil {
		return err
	}
	fmt.Printf("Open this link to set up PatchCaster. It works once, until %s.\n\n  %s\n",
		expires.Format(time.RFC3339), link.URL)
	return nil
}

// AdminCmd groups the recovery of administrator access (SYSREQ-038).
type AdminCmd struct {
	Grant AdminGrantCmd `cmd:"" help:"Make a person of the identity provider an administrator."`
}

// AdminGrantCmd gives the administrator role to an identity, whether or not
// it ever logged in. It needs the database credentials, which already give
// full control of the instance (ADR-033).
type AdminGrantCmd struct {
	DatabaseURL string `flag:"" required:"" env:"PATCHCASTER_DATABASE_URL" help:"PostgreSQL connection URL."`
	Issuer      string `flag:"" required:"" help:"Issuer of the identity provider, exactly as its ID tokens name it."`
	Subject     string `flag:"" required:"" help:"Subject of the person, exactly as their ID tokens name it."`
}

// Run grants the role and records the grant in the audit log.
func (c *AdminGrantCmd) Run(ctx context.Context) error {
	st, closeDB, err := openMigratedStore(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer closeDB()

	actor := store.Actor{Label: "admin grant command"}
	if err := st.GrantAdministrator(ctx, actor, c.Issuer, c.Subject, time.Now()); err != nil {
		return err
	}
	newLogger("info").Info("administrator role granted", "issuer", c.Issuer, "subject", c.Subject)
	return nil
}
