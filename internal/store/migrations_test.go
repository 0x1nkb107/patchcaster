package store_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// openEmptyDatabase connects to the test container and recreates its public
// schema, so the test starts without any table.
func openEmptyDatabase(t *testing.T) *sql.DB {
	t.Helper()
	cfg, err := pgx.ParseConfig(databaseURL)
	require.NoError(t, err)
	db := pgxstdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ExecContext(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err)
	return db
}

// applyV01Schema builds the state a v0.1 release leaves behind: its frozen
// migrations, recorded in goose's default history table as v0.1 did.
func applyV01Schema(t *testing.T, db *sql.DB) {
	t.Helper()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("testdata/v01-migrations"))
	require.NoError(t, err)
	_, err = provider.Up(context.Background())
	require.NoError(t, err)
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow("SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists))
	return exists
}

// A v0.1 database is refused before anything is written (SYSREQ-043).
func TestMigrateRefusesAV01Database(t *testing.T) {
	// arrange
	db := openEmptyDatabase(t)
	applyV01Schema(t, db)

	// act
	applied, err := store.Migrate(context.Background(), db)

	// assert
	require.ErrorIs(t, err, store.ErrV01Database)
	assert.Zero(t, applied)
	assert.False(t, tableExists(t, db, "patchcaster_migrations"), "the refusal must not start a v0.2 history")
}

// A fresh database records its history in v0.2's own table, and its own
// tables never trigger the v0.1 refusal on a later start.
func TestMigrateStartsANewHistory(t *testing.T) {
	// arrange
	db := openEmptyDatabase(t)
	ctx := context.Background()

	// act
	first, firstErr := store.Migrate(ctx, db)
	again, againErr := store.Migrate(ctx, db)

	// assert
	require.NoError(t, firstErr)
	require.NoError(t, againErr, "a v0.2 database must not be mistaken for a v0.1 one")
	assert.Positive(t, first)
	assert.Zero(t, again, "a second run has nothing left to apply")
	assert.True(t, tableExists(t, db, "patchcaster_migrations"))
	assert.False(t, tableExists(t, db, "goose_db_version"))
}
