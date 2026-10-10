-- name: Ping :one
SELECT 1;

-- name: SeenExists :one
SELECT EXISTS (SELECT 1 FROM seen_webhook WHERE webhook_id = $1);

-- name: MarkSeen :execresult
INSERT INTO seen_webhook (webhook_id, outcome, first_seen)
VALUES ($1, $2, $3)
ON CONFLICT (webhook_id) DO NOTHING;

-- name: InsertOutbox :exec
INSERT INTO outbox (mr_ref, action, dest_channel, payload, state, attempts,
                    next_attempt_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'pending', 0, $5, $5, $5);

-- name: PruneSeen :exec
DELETE FROM seen_webhook WHERE first_seen < $1;

-- name: ClaimDue :one
UPDATE outbox
SET claimed_at = $1, lease_expiry = $2, state = 'claimed', updated_at = $1
WHERE id = (
    SELECT id FROM outbox
    WHERE state = 'pending' AND next_attempt_at <= $1
    ORDER BY id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, mr_ref, action, dest_channel, payload, attempts;

-- name: ReapLeases :exec
UPDATE outbox
SET state = 'pending', claimed_at = NULL, lease_expiry = NULL
WHERE state = 'claimed' AND lease_expiry < $1;

-- name: SettleSent :exec
UPDATE outbox
SET state = 'sent', settled_at = $2, settled_reason = NULL, updated_at = $2
WHERE id = $1;

-- name: SettleSkip :exec
UPDATE outbox
SET state = 'skipped', settled_reason = $2, settled_at = $3, updated_at = $3,
    claimed_at = NULL, lease_expiry = NULL
WHERE id = $1;

-- name: SettleRetry :exec
UPDATE outbox
SET state = 'pending', attempts = $2, next_attempt_at = $3, last_error = $4,
    claimed_at = NULL, lease_expiry = NULL, updated_at = $3
WHERE id = $1;

-- name: SettlePark :exec
UPDATE outbox
SET state = 'parked', attempts = $2, settled_reason = $3, settled_at = $4,
    last_error = $5, claimed_at = NULL, lease_expiry = NULL, updated_at = $4
WHERE id = $1;

-- name: ThreadGet :one
SELECT mr_ref, channel_id, root_post_id, state FROM mr_thread WHERE mr_ref = $1;

-- name: ThreadOpen :exec
INSERT INTO mr_thread (mr_ref, channel_id, root_post_id, state, created_at, updated_at)
VALUES ($1, $2, $3, 'open', $4, $4)
ON CONFLICT (mr_ref) DO NOTHING;

-- name: ThreadClose :exec
UPDATE mr_thread
SET state = 'closed', updated_at = $2
WHERE mr_ref = $1;
