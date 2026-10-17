// Command patchcaster forwards the merge request events of forges into chat
// channels, as one thread per merge request and channel.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// version is stamped at link time by the release pipeline
// (-ldflags -X main.version, ADR-015); source builds report "dev".
var version = "dev"

// CLI is the kong root. Running bare "patchcaster" serves. Every setting is a
// flag with a PATCHCASTER_-prefixed environment twin (ADR-003/ADR-012).
// Precedence is flags > environment variables > built-in defaults, and any
// invalid value exits non-zero before the service listens (SYSREQ-022).
type CLI struct {
	Serve     ServeCmd     `cmd:"" default:"withargs" help:"Run the webhook relay."`
	Migrate   MigrateCmd   `cmd:"" help:"Apply database migrations and exit."`
	Bootstrap BootstrapCmd `cmd:"" help:"Issue the one-time setup link that creates the first administrator."`
	Admin     AdminCmd     `cmd:"" help:"Restore administrator access."`
	Secrets   SecretsCmd   `cmd:"" help:"Manage the secrets PatchCaster stores."`

	Version kong.VersionFlag `help:"Show the build version and exit."`
}

func main() {
	// SIGTERM/SIGINT start the documented drain (SYSREQ-023); without this
	// trap the process would die at the default disposition, code 143.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var cli CLI
	kctx := kong.Parse(&cli, kongOptions(ctx)...)

	// The commands' Run methods return errors instead of exiting, so the
	// wiring stays testable; kong reports the error and exits non-zero
	// (SYSREQ-022: invalid configuration never starts listening).
	kctx.FatalIfErrorf(kctx.Run())
}

// kongOptions configures the command line for main and the tests alike. ctx
// reaches every command's Run method as its context.Context.
func kongOptions(ctx context.Context) []kong.Option {
	return []kong.Option{
		kong.Name("patchcaster"),
		kong.Description("Forward merge request events of forges into chat threads."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
		kong.BindTo(ctx, (*context.Context)(nil)),
	}
}

// MigrateCmd applies goose migrations then exits, for migration-before-rollout
// pipelines (SYSREQ-021 posture).
type MigrateCmd struct {
	DatabaseURL string `flag:"" required:"" env:"PATCHCASTER_DATABASE_URL" help:"PostgreSQL connection URL."`
}

// Run applies the embedded migrations over a connection it opens and closes.
func (c *MigrateCmd) Run(ctx context.Context) error {
	db, err := openDB(c.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return migrate(ctx, db, newLogger("info"))
}

// openDB opens a pooled database handle through the pure-Go pgx stdlib
// driver: CGO stays off end to end (ADR-010).
func openDB(url string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	db := pgxstdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(16)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

// migrate brings the schema up to the embedded revision and reports progress
// through log. The mechanism lives in the store package (SYSARCH-002: the
// engine story stays behind its boundary); db ownership stays with the caller.
func migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	applied, err := store.Migrate(ctx, db)
	if err != nil {
		return err
	}
	log.Info("migrations applied", "count", applied)
	return nil
}

// openMigratedStore opens the database for a one-off command and migrates it
// first, like serve, so that the command always meets the schema it was
// built for. The returned function closes the database.
func openMigratedStore(ctx context.Context, url string) (*store.Postgres, func(), error) {
	db, err := openDB(url)
	if err != nil {
		return nil, nil, err
	}
	closeDB := func() { _ = db.Close() }
	if err := migrate(ctx, db, newLogger("info")); err != nil {
		closeDB()
		return nil, nil, err
	}
	return store.New(db), closeDB, nil
}

// SecretsCmd groups the maintenance of stored secrets (ADR-035).
type SecretsCmd struct {
	Reseal ResealCmd `cmd:"" help:"Seal every stored secret again with the active encryption key, after a key rotation."`
}

// ResealCmd moves every stored secret to the active encryption key, so that
// a previous key can be dropped afterwards.
type ResealCmd struct {
	DatabaseURL string   `flag:"" required:"" env:"PATCHCASTER_DATABASE_URL" help:"PostgreSQL connection URL."`
	Keys        KeyFlags `embed:""`
}

// Validate rejects malformed keys before anything connects.
func (c *ResealCmd) Validate() error {
	return c.Keys.validate()
}

// Run reseals over a connection it opens and closes.
func (c *ResealCmd) Run(ctx context.Context) error {
	box, err := c.Keys.box()
	if err != nil {
		return err
	}
	st, closeDB, err := openMigratedStore(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer closeDB()
	resealed, err := resealSecrets(ctx, st, box, time.Now())
	if err != nil {
		return err
	}
	newLogger("info").Info("secrets resealed", "count", resealed)
	return nil
}
