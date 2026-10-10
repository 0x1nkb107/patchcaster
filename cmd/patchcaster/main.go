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

	"github.com/0x5a17ed/patchcaster/internal/store"
)

// version is stamped at link time by the release pipeline
// (-ldflags -X main.version, ADR-015); source builds report "dev".
var version = "dev"

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("patchcaster"),
		kong.Description("Forward GitLab merge request webhooks to Mattermost threads."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)

	// SIGTERM/SIGINT start the documented drain (SYSREQ-023); without this
	// trap the process would die at the default disposition, code 143.
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := dispatch(runCtx, ctx.Command(), &cli); err != nil {
		fatal(err)
	}
}

// dispatch keeps Run free of os.Exit so the wiring is testable; main converts
// any error into a non-zero exit (SYSREQ-022: invalid configuration never
// starts listening).
func dispatch(ctx context.Context, command string, cli *CLI) error {
	switch command {
	case "migrate":
		db, err := openDB(cli.Migrate.DatabaseURL)
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		return migrate(ctx, db, newLogger("info"))
	case "serve":
		return serve(ctx, &cli.Serve)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
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

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "patchcaster: %v\n", err)
	os.Exit(1)
}
