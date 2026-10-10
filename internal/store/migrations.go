package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// migrationsFS carries the goose SQL files inside the binary so both
// "patchcaster migrate" and serve-time migration use the exact schema the
// binary was compiled with (SYSREQ-021). It is package-private: the provider
// machinery below is the only reader, keeping engine detail inside the store
// boundary (SYSARCH-002).
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate brings the database schema up to the embedded revision and reports
// how many migrations it applied.
//
// The session locker (goose's recommended Postgres coordination) serializes
// concurrent runs: rolling-update overlap or an operator invoking
// "patchcaster migrate" while a pod starts would otherwise race identical DDL,
// the loser aborting mid-transaction. If the lock connection dies, PostgreSQL
// drops the advisory lock and rolls back the migration transaction, so the
// next runner redoes it cleanly.
//
// The provider borrows db: goose.Provider.Close is exactly db.Close, meant for
// callers that hand over ownership. Here the pool outlives migration and its
// owner keeps closing it — do not close a provider around this call.
func Migrate(ctx context.Context, db *sql.DB) (int, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return 0, fmt.Errorf("migration session locker: %w", err)
	}
	// The provider globs only the top level of fsys (no recursion), so it
	// must see the migrations directory itself, not the embed root.
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return 0, fmt.Errorf("embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations,
		goose.WithSessionLocker(locker))
	if err != nil {
		return 0, fmt.Errorf("goose provider: %w", err)
	}
	applied, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	return len(applied), nil
}
