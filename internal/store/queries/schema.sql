-- name: HoldsV01State :one
-- HoldsV01State reports whether the database carries the state of a v0.1
-- release: goose's default history table next to v0.1's thread table.
-- v0.2 records its own history under another name, so its databases never
-- match (SYSREQ-043).
SELECT (to_regclass('goose_db_version') IS NOT NULL
    AND to_regclass('mr_thread') IS NOT NULL)::boolean AS holds_v01;
