-- name: SeenExists :one
-- SeenExists reports whether a webhook identifier was already recorded
-- (SYSREQ-014).
SELECT EXISTS (
    SELECT 1
    FROM seen_webhook
    WHERE webhook_id = $1
);

-- name: MarkSeen :execresult
-- MarkSeen records a webhook identifier with its outcome. A duplicate
-- inserts nothing, so the affected row count tells the first delivery apart.
INSERT INTO seen_webhook (webhook_id, outcome, first_seen)
VALUES ($1, $2, $3)
ON CONFLICT (webhook_id) DO NOTHING;

-- name: PruneSeen :exec
-- PruneSeen deletes the identifiers recorded before the retention horizon.
DELETE FROM seen_webhook
WHERE first_seen < $1;
