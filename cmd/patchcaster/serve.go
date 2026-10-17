package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	z "github.com/Oudwins/zog"
	"github.com/lmittmann/tint"

	"github.com/patchcaster-app/patchcaster/internal/api"
	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/deliver"
	"github.com/patchcaster-app/patchcaster/internal/graceful"
	"github.com/patchcaster-app/patchcaster/internal/ingest"
	"github.com/patchcaster-app/patchcaster/internal/metrics"
	"github.com/patchcaster-app/patchcaster/internal/ops"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ServeCmd is the running configuration.
type ServeCmd struct {
	Addr           string        `flag:"" default:":8080" env:"PATCHCASTER_ADDR" help:"Public listener address (webhook endpoint)."`
	InternalAddr   string        `flag:"" default:":9090" env:"PATCHCASTER_INTERNAL_ADDR" help:"Internal listener address (probes, metrics)."`
	ManagementAddr string        `flag:"" default:":8081" env:"PATCHCASTER_MANAGEMENT_ADDR" help:"Management listener address (API and web interface)."`
	InterfaceURL   string        `flag:"" required:"" env:"PATCHCASTER_INTERFACE_URL" help:"External address of the web interface, such as https://patchcaster.example.org."`
	PublicURL      string        `flag:"" required:"" env:"PATCHCASTER_PUBLIC_URL" help:"External address of the webhook listener, from which the forges' webhook endpoints are built."`
	DatabaseURL    string        `flag:"" required:"" env:"PATCHCASTER_DATABASE_URL" help:"PostgreSQL connection URL."`
	DrainTimeout   time.Duration `flag:"" default:"30s" env:"PATCHCASTER_DRAIN_TIMEOUT" help:"In-flight budget after SIGTERM; the deployment grace period must exceed it."`
	EventMaxAge    time.Duration `flag:"" default:"4h" env:"PATCHCASTER_EVENT_MAX_AGE" help:"Freshness cap; older verified events are discarded."`
	SeenRetention  time.Duration `flag:"" default:"24h" env:"PATCHCASTER_SEEN_RETENTION" help:"Seen-webhook retention; GitLab retries stay inside it."`
	ClaimLease     time.Duration `flag:"" default:"60s" env:"PATCHCASTER_CLAIM_LEASE" help:"Job claim lease before reaping."`
	PollInterval   time.Duration `flag:"" default:"10s" env:"PATCHCASTER_POLL_INTERVAL" help:"Fallback claim interval for retries, expired leases and missed wake-ups."`
	LogLevel       string        `flag:"" default:"info" env:"PATCHCASTER_LOG_LEVEL" help:"Log level: debug|info|warn|error."`

	Keys KeyFlags `embed:""`
}

// serveSchema asserts the semantic constraints on kong-parsed values
// (ADR-012). Keys are ServeCmd field names; Validate renders them
// back into operator-facing flag names.
var serveSchema = z.Struct(z.Shape{
	"LogLevel":      z.String().OneOf([]string{"debug", "info", "warn", "error"}),
	"DrainTimeout":  positiveDuration(),
	"EventMaxAge":   positiveDuration(),
	"SeenRetention": positiveDuration(),
	"ClaimLease":    positiveDuration(),
	"PollInterval":  positiveDuration(),
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
	"LogLevel":      "--log-level",
	"DrainTimeout":  "--drain-timeout",
	"EventMaxAge":   "--event-max-age",
	"SeenRetention": "--seen-retention",
	"ClaimLease":    "--claim-lease",
	"PollInterval":  "--poll-interval",
}

// Validate enforces the startup invariants; kong calls it on the selected
// command after parsing, before Run. Every violation is reported at once
// (ADR-012), so a misconfigured first deploy names all offending options.
func (c *ServeCmd) Validate() error {
	issues := append(serveSchema.Validate(c), replayLoopSchema.Validate(c)...)
	errs := make([]error, 0, len(issues)+1)
	for _, issue := range issues {
		errs = append(errs, fmt.Errorf("%s: %s", flagName(issue.Path), issue.Message))
	}
	if err := c.Keys.validate(); err != nil {
		errs = append(errs, err)
	}
	if c.InterfaceURL != "" {
		if _, err := parseInterfaceURL(c.InterfaceURL); err != nil {
			errs = append(errs, err)
		}
	}
	if c.PublicURL != "" {
		if _, err := parsePublicURL(c.PublicURL); err != nil {
			errs = append(errs, err)
		}
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

// Run serves until ctx ends. It wires every component and hands them to the
// lifecycle, which owns the shutdown choreography (SYSREQ-023). The
// deployment's terminationGracePeriodSeconds must exceed --drain-timeout.
func (c *ServeCmd) Run(ctx context.Context) error {
	log := newLogger(c.LogLevel)
	// The login's relying party logs through the default logger (ADR-033).
	slog.SetDefault(log)

	interfaceURL, err := parseInterfaceURL(c.InterfaceURL)
	if err != nil {
		return err
	}
	publicURL, err := parsePublicURL(c.PublicURL)
	if err != nil {
		return err
	}

	db, err := openDB(c.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("reach database: %w", err)
	}
	if err := migrate(ctx, db, log); err != nil {
		return err
	}

	st := store.New(db)
	box, err := c.Keys.box()
	if err != nil {
		return err
	}
	stale, err := verifyInstanceKey(ctx, st, box, time.Now())
	if err != nil {
		return err
	}
	if stale {
		log.Warn("stored secrets are sealed with a previous encryption key: run patchcaster secrets reseal")
	}
	m := metrics.New()
	probe := ops.NewServer(st, m)

	renderer, err := deliver.NewRenderer()
	if err != nil {
		return err
	}
	// Ingest and delivery read the stored configuration for every event and
	// message, so a committed change applies without a restart (SYSREQ-034).
	handler := &ingest.Handler{
		Keys:    ingest.Keyring{Store: st, Box: box},
		Store:   st,
		MaxAge:  c.EventMaxAge,
		Now:     time.Now,
		Log:     log,
		Metrics: m,
	}
	listener, err := store.NewOutboxListener(c.DatabaseURL)
	if err != nil {
		return err
	}
	wakeup := deliver.NewWakeup(listener, m, log)
	worker := deliver.NewWorker(
		st,
		deliver.ConnectionPosters{Store: st, Box: box},
		renderer, m, log,
		deliver.Config{Poll: c.PollInterval, Lease: c.ClaimLease, Wake: wakeup.C()},
		nil,
	)

	sessions := auth.NewSessions(st.Sessions(), interfaceURL.Scheme == "https")
	login := auth.NewLogin(st, box, sessions, interfaceURL, log)
	svc := api.NewService(api.Config{
		Version:   version,
		Store:     st,
		Box:       box,
		Sessions:  sessions,
		Login:     login,
		Platforms: platform.Checker{},
		PublicURL: publicURL,
		Log:       log,
	})
	management, err := managementHandler(svc, login, sessions, interfaceURL)
	if err != nil {
		return fmt.Errorf("build management listener: %w", err)
	}

	// Bind before announcing readiness: a port conflict must fail the start,
	// not arrive as a late listener error.
	pubLn, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("bind public listener %s: %w", c.Addr, err)
	}
	intLn, err := net.Listen("tcp", c.InternalAddr)
	if err != nil {
		_ = pubLn.Close()
		return fmt.Errorf("bind internal listener %s: %w", c.InternalAddr, err)
	}
	mgmtLn, err := net.Listen("tcp", c.ManagementAddr)
	if err != nil {
		_ = pubLn.Close()
		_ = intLn.Close()
		return fmt.Errorf("bind management listener %s: %w", c.ManagementAddr, err)
	}

	lc := &lifecycle{
		internal: func(ctx context.Context) error {
			return graceful.Serve(ctx, intLn, probe.Mux(), c.DrainTimeout)
		},
		public: func(ctx context.Context) error {
			return graceful.Serve(ctx, pubLn, handler.Mux(), c.DrainTimeout)
		},
		management: func(ctx context.Context) error {
			return graceful.Serve(ctx, mgmtLn, management, c.DrainTimeout)
		},
		worker: worker,
		background: []func(context.Context) error{
			func(ctx context.Context) error { return pruneLoop(ctx, st, c.SeenRetention, log) },
			func(ctx context.Context) error { return expireSessionsLoop(ctx, st, log) },
			wakeup.Run,
		},
		markReady:   probe.MarkReady,
		drainBudget: c.DrainTimeout,
		log:         log,
	}
	log.Info("serving", "public", c.Addr, "internal", c.InternalAddr, "management", c.ManagementAddr)
	return lc.run(ctx)
}

// pruneLoop keeps the dedupe table bounded: seen rows older than the
// retention horizon no longer protect anything (SYSREQ-014).
func pruneLoop(ctx context.Context, st *store.Postgres, retention time.Duration, log *slog.Logger) error {
	prune := func() {
		if err := st.PruneSeen(ctx, time.Now().Add(-retention)); err != nil {
			log.Error("prune seen webhooks failed", "err", err)
		}
	}
	prune()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			prune()
		}
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	h := tint.NewTextHandler(os.Stderr, &tint.Options{
		Level:      lvl,
		NoColor:    true, // log output is piped in production; color buys nothing
		TimeFormat: time.RFC3339,
	})
	return slog.New(h)
}
