// Package config defines the command line and environment surface
// (ADR-003/ADR-012). Precedence is flags > PATCHCASTER_-prefixed environment
// variables > built-in defaults, and any invalid value exits non-zero before
// the service listens (SYSREQ-022).
package main

import (
	"errors"
	"fmt"
	"time"

	z "github.com/Oudwins/zog"
	"github.com/alecthomas/kong"
)

// CLI is the kong root. Running bare "patchcaster" serves.
type CLI struct {
	Serve   ServeCmd   `cmd:"" default:"withargs" help:"Run the webhook relay."`
	Migrate MigrateCmd `cmd:"" help:"Apply database migrations and exit."`

	Version kong.VersionFlag `help:"Show the build version and exit."`
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

// serveSchema asserts the semantic constraints on kong-parsed values
// (ADR-012). Keys are ServeCmd field names; validationIssues renders them
// back into operator-facing flag names.
var serveSchema = z.Struct(z.Shape{
	// Required, not Min(1): non-required schemas skip their tests on zero
	// values, so only Required could catch an empty rotation entry — belt and
	// braces, since kong was observed pruning empty segments at the separator.
	"SigningSecrets": z.Slice(z.String().Required(z.Message("signing secret must not be empty"))).Required(),
	"LogLevel":       z.String().OneOf([]string{"debug", "info", "warn", "error"}),
	"DrainTimeout":   positiveDuration(),
	"EventMaxAge":    positiveDuration(),
	"SeenRetention":  positiveDuration(),
	"ClaimLease":     positiveDuration(),
})

// replayLoopSchema keeps the replay loop closed: dedupe must outlive the
// freshness cap, or a redelivery is neither suppressed nor expired
// (SYSREQ-013/014). v0.23 has no struct-level tests (ADR-012), so the rule is
// a whole-struct custom schema that attaches its issue to SeenRetention.
var replayLoopSchema = z.CustomFunc(func(c *ServeCmd, ctx z.Ctx) bool {
	if c.SeenRetention <= c.EventMaxAge {
		ctx.AddIssue(ctx.Issue().
			SetPath([]string{"SeenRetention"}).
			SetMessage(fmt.Sprintf("must exceed --event-max-age (%s)", c.EventMaxAge)))
	}
	// Always true: the manual issue is the report; returning false would add
	// a redundant generic auto-issue on top of it.
	return true
})

// positiveDuration asserts > 0; v0.23 ships no native duration schema.
func positiveDuration() *z.Custom[time.Duration] {
	return z.CustomFunc(func(d *time.Duration, _ z.Ctx) bool {
		return *d > 0
	}, z.Message("must be a positive duration"))
}

// flagNames renders schema field paths as the flags operators typed.
var flagNames = map[string]string{
	"SigningSecrets": "--signing-secret",
	"LogLevel":       "--log-level",
	"DrainTimeout":   "--drain-timeout",
	"EventMaxAge":    "--event-max-age",
	"SeenRetention":  "--seen-retention",
	"ClaimLease":     "--claim-lease",
}

// Validate enforces the startup invariants; kong calls it on the selected
// command after parsing, before Run. Every violation is reported at once
// (ADR-012), so a misconfigured first deploy names all offending options.
func (c *ServeCmd) Validate() error {
	issues := append(serveSchema.Validate(c), replayLoopSchema.Validate(c)...)
	if len(issues) == 0 {
		return nil
	}
	errs := make([]error, 0, len(issues))
	for _, issue := range issues {
		errs = append(errs, fmt.Errorf("%s: %s", flagName(issue.Path), issue.Message))
	}
	return errors.Join(errs...)
}

func flagName(path []string) string {
	field := "configuration"
	if len(path) > 0 {
		field = path[0]
	}
	if flag, ok := flagNames[field]; ok {
		return flag
	}
	return field
}
