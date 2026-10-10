// Package config defines the command line and environment surface
// (ADR-004/ADR-008). Precedence is flags > PATCHCASTER_-prefixed environment
// variables > built-in defaults, and any invalid value exits non-zero before
// the service listens (SYSREQ-022).
package main

import (
	"fmt"
	"time"
)

// CLI is the kong root. Running bare "patchcaster" serves.
type CLI struct {
	Serve   ServeCmd   `cmd:"" default:"withargs" help:"Run the webhook relay."`
	Migrate MigrateCmd `cmd:"" help:"Apply database migrations and exit."`
}

// MigrateCmd applies goose migrations then exits, for migration-before-rollout
// pipelines (SYSREQ-021 posture).
type MigrateCmd struct {
	DatabaseURL string `flag:"" required:"" env:"PATCHCASTER_DATABASE_URL" help:"PostgreSQL connection URL."`
}

// ServeCmd is the running configuration.
type ServeCmd struct {
	Addr            string        `flag:"" default:":8080" env:"PATCHCASTER_ADDR" help:"Public listener address (webhook endpoint)."`
	InternalAddr    string        `flag:"" default:":9090" env:"PATCHCASTER_INTERNAL_ADDR" help:"Internal listener address (probes, metrics)."`
	DatabaseURL     string        `flag:"" required:"" env:"PATCHCASTER_DATABASE_URL" help:"PostgreSQL connection URL."`
	MattermostURL   string        `flag:"" required:"" env:"PATCHCASTER_MATTERMOST_URL" help:"Mattermost site URL."`
	MattermostToken string        `flag:"" required:"" env:"PATCHCASTER_MATTERMOST_TOKEN" help:"Bot account auth token."`
	SigningSecrets  []string      `flag:"" name:"signing-secret" sep:"," env:"PATCHCASTER_SIGNING_SECRETS" required:"" help:"GitLab Standard Webhooks signing secret; repeat or comma-separate for rotation."`
	RoutingFile     string        `flag:"" required:"" env:"PATCHCASTER_ROUTING_FILE" help:"Routing table YAML."`
	DrainTimeout    time.Duration `flag:"" default:"30s" env:"PATCHCASTER_DRAIN_TIMEOUT" help:"In-flight budget after SIGTERM; deployment grace period must exceed it (SYSREQ-023)."`
	EventMaxAge     time.Duration `flag:"" default:"4h" env:"PATCHCASTER_EVENT_MAX_AGE" help:"Freshness cap; older verified events are discarded (SYSREQ-013)."`
	SeenRetention   time.Duration `flag:"" default:"24h" env:"PATCHCASTER_SEEN_RETENTION" help:"Seen-webhook retention; GitLab retries stay inside it (SYSREQ-014)."`
	ClaimLease      time.Duration `flag:"" default:"60s" env:"PATCHCASTER_CLAIM_LEASE" help:"Job claim lease before reaping."`
	LogLevel        string        `flag:"" default:"info" env:"PATCHCASTER_LOG_LEVEL" help:"Log level: debug|info|warn|error."`
}

// Validate enforces the startup invariants. kong calls it on the selected
// command after parsing, before Run.
func (c *ServeCmd) Validate() error {
	if len(c.SigningSecrets) == 0 {
		return fmt.Errorf("--signing-secret must carry at least one secret")
	}
	for i, s := range c.SigningSecrets {
		if s == "" {
			return fmt.Errorf("--signing-secret entry %d is empty", i+1)
		}
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("--log-level %q is not debug|info|warn|error", c.LogLevel)
	}
	if c.DrainTimeout <= 0 {
		return fmt.Errorf("--drain-timeout must be positive")
	}
	if c.EventMaxAge <= 0 {
		return fmt.Errorf("--event-max-age must be positive")
	}
	if c.ClaimLease <= 0 {
		return fmt.Errorf("--claim-lease must be positive")
	}
	// The replay loop closes only while dedupe outlives the freshness cap:
	// a redelivery either still dedupes or is now old enough to discard
	// (SYSREQ-013/014).
	if c.SeenRetention <= c.EventMaxAge {
		return fmt.Errorf("--seen-retention (%s) must exceed --event-max-age (%s)", c.SeenRetention, c.EventMaxAge)
	}
	return nil
}
