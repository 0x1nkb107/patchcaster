// Command patchcaster forwards GitLab merge request webhook events to
// Mattermost channels as one thread per merge request.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/0x5a17ed/patchcaster/internal/store"
)

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("patchcaster"),
		kong.Description("Forward GitLab merge request webhooks to Mattermost threads."),
		kong.UsageOnError(),
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

// migrate brings the schema up to the embedded revision. The session locker
// (goose's recommended Postgres coordination) serializes concurrent runs:
// rolling-update overlap or an operator invoking `migrate` while a pod starts
// would otherwise race identical DDL — the loser would abort mid-transaction.
// If the lock connection dies, PostgreSQL drops the advisory lock and rolls
// back the migration transaction, so the next runner redoes it cleanly.
func migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("migration session locker: %w", err)
	}
	// The provider globs only the top level of fsys (no recursion), so it
	// must see the migrations directory itself, not the embed root.
	migrations, err := fs.Sub(store.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrations,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	// The provider borrows db: Provider.Close is exactly db.Close, meant for
	// callers that hand over ownership. Here the pool outlives migration
	// (serve keeps serving), and both callers close it (serve.go defer,
	// dispatch in main.go). Do not close the provider — IDE resource-leak
	// warnings here are the by-caller-close pattern, not a leak.
	applied, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	log.Info("migrations applied", "count", len(applied))
	return nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "patchcaster: %v\n", err)
	os.Exit(1)
}
