package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// historyTable records the applied migrations of v0.2 and later. v0.1 kept
// its history in goose's default table. A table of its own starts a new
// history, so a v0.1 database can never pass for a v0.2 one (SYSREQ-043).
const historyTable = "patchcaster_migrations"

// ErrV01Database reports a database that still holds the state of a v0.1
// release. v0.2 is an incompatible upgrade and needs a new, empty database
// (SYSREQ-043).
var ErrV01Database = errors.New("database holds PatchCaster v0.1 state, and v0.2 needs a new, empty database")

// migrationsFS carries the goose SQL files inside the binary so both
// "patchcaster migrate" and serve-time migration use the exact schema the
// binary was compiled with (SYSREQ-021). It is package-private: the provider
// machinery below is the only reader, keeping engine detail inside the store
// boundary (SYSARCH-002).
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate brings the database schema up to the embedded revision and reports
// how many migrations it applied. A database that holds v0.1 state is
// refused with ErrV01Database before anything is written.
//
// The session locker (goose's recommended Postgres coordination) serializes
// concurrent runs: rolling-update overlap or the infrastructure team invoking
// "patchcaster migrate" while a pod starts would otherwise race identical DDL,
// the loser aborting mid-transaction. If the lock connection dies, PostgreSQL
// drops the advisory lock and rolls back the migration transaction, so the
// next runner redoes it cleanly.
//
// The provider borrows db: goose.Provider.Close is exactly db.Close, meant for
// callers that hand over ownership. Here the pool outlives migration and its
// owner keeps closing it — do not close a provider around this call.
func Migrate(ctx context.Context, db *sql.DB) (int, error) {
	if err := refuseV01State(ctx, db); err != nil {
		return 0, err
	}
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
		goose.WithSessionLocker(locker), goose.WithTableName(historyTable))
	if err != nil {
		return 0, fmt.Errorf("goose provider: %w", err)
	}
	applied, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	return len(applied), nil
}

// refuseV01State stops a migration before it writes anything when the
// database still holds v0.1 state. v0.1 identified a merge request by
// GitLab's global id and stored no target repository, so its threads cannot
// carry over to v0.2 (SYSREQ-043).
func refuseV01State(ctx context.Context, db *sql.DB) error {
	holds, err := sqlcgen.New(db).HoldsV01State(ctx)
	if err != nil {
		return fmt.Errorf("check for v0.1 state: %w", err)
	}
	if holds {
		return ErrV01Database
	}
	return nil
}
