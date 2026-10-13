package e2e

import (
	"database/sql"
	"errors"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx driver for database/sql
	"github.com/stretchr/testify/require"
)

// freshDatabase recreates the public schema of the suite's container, so
// that the test starts from an empty store, and returns a handle for reading
// the outcome. The service applies its migrations itself when it starts.
func freshDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err)
	return db
}

// outboxRow is one queued delivery as the tests inspect it.
type outboxRow struct {
	action   string
	state    string
	attempts int
}

// outboxRows lists the queued deliveries in arrival order.
func outboxRows(t *testing.T, db *sql.DB) []outboxRow {
	t.Helper()
	rows, err := db.Query("SELECT action, state, attempts FROM outbox ORDER BY id")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var out []outboxRow
	for rows.Next() {
		var row outboxRow
		require.NoError(t, rows.Scan(&row.action, &row.state, &row.attempts))
		out = append(out, row)
	}
	require.NoError(t, rows.Err())
	return out
}

// outboxStates lists the states of the queued deliveries in arrival order.
func outboxStates(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var states []string
	for _, row := range outboxRows(t, db) {
		states = append(states, row.state)
	}
	return states
}

// seenWebhooks counts the recorded webhook identifiers.
func seenWebhooks(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM seen_webhook").Scan(&count))
	return count
}

// threadRoot returns the root post mapped to a merge request, or an empty
// string when no thread is mapped.
func threadRoot(t *testing.T, db *sql.DB, mrRef string) string {
	t.Helper()
	var root string
	err := db.QueryRow("SELECT root_post_id FROM mr_thread WHERE mr_ref = $1", mrRef).Scan(&root)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	require.NoError(t, err)
	return root
}
