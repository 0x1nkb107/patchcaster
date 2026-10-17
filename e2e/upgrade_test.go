package e2e

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v01State is the part of a v0.1 database that v0.2 recognizes: goose's
// default history table with v0.1's three migrations, next to v0.1's thread
// table holding a live thread.
const v01State = `
CREATE TABLE goose_db_version (
    id SERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL,
    is_applied BOOLEAN NOT NULL,
    tstamp TIMESTAMP NOT NULL DEFAULT now()
);
INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, true), (1, true), (2, true), (3, true);
CREATE TABLE mr_thread (
    mr_ref TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL,
    root_post_id TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
INSERT INTO mr_thread VALUES ('1234', 'channel', 'root', 'open', now(), now());
`

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow("SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists))
	return exists
}

// Started against a v0.1 database, the service refuses to start, says why
// and what to do, and leaves the database as it found it (SCEN-017).
func TestAV01DatabaseIsRefused(t *testing.T) {
	// arrange
	db := freshDatabase(t)
	_, err := db.Exec(v01State)
	require.NoError(t, err)

	// act
	svc := launch(t, serviceConfig{})
	code := svc.awaitExit(t, 15*time.Second)

	// assert
	assert.NotZero(t, code, "the service must not run on a v0.1 database")
	assert.Contains(t, svc.logs.String(), "v0.2 needs a new, empty database")
	assert.False(t, tableExists(t, db, "patchcaster_migrations"), "the refusal must write nothing")
	assert.Equal(t, "root", threadRoot(t, db, "1234"), "the v0.1 thread must stay untouched")
}
