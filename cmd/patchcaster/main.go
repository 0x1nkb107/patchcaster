// Command patchcaster forwards GitLab merge request webhook events to
// Mattermost channels as one thread per merge request.
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
		kong.Description("Forward GitLab merge request webhooks to Mattermost threads."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
		kong.BindTo(ctx, (*context.Context)(nil)),
	}
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
