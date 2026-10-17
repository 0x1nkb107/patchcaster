-- name: InsertOutbox :exec
-- InsertOutbox enqueues a delivery as pending and due at once (SYSREQ-017).
INSERT INTO outbox (mr_ref, sink_id, connection_id, action, dest_channel, payload,
    state, attempts, next_attempt_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, 'pending', 0, $7, $7, $7);

-- name: NotifyOutbox :exec
-- NotifyOutbox wakes listening workers once the enqueuing transaction
-- commits. The notification carries no payload (ADR-019).
SELECT pg_notify(sqlc.arg(channel)::text, '');

-- name: ClaimDue :one
-- ClaimDue claims the oldest due row under a lease. A row waits while an
-- older row of its merge request and sink is still pending or claimed: a
-- retrying or in-flight delivery holds back the rest of its thread, so
-- replies never overtake their root (SYSARCH-002). Other sinks of the same
-- merge request never wait for it (SYSREQ-048). Parked and skipped rows are
-- settled and hold nothing back.
UPDATE outbox
SET claimed_at = $1, lease_expiry = $2, state = 'claimed', updated_at = $1
WHERE id = (
    SELECT candidate.id
    FROM outbox AS candidate
    WHERE candidate.state = 'pending' AND candidate.next_attempt_at <= $1
        AND NOT EXISTS (
            SELECT 1
            FROM outbox AS earlier
            WHERE earlier.mr_ref = candidate.mr_ref
                AND earlier.sink_id = candidate.sink_id
                AND earlier.id < candidate.id
                AND earlier.state IN ('pending', 'claimed')
        )
    ORDER BY candidate.id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, mr_ref, sink_id, connection_id, action, dest_channel, payload,
    attempts, notice_posted, root_refreshed;

-- name: ReapLeases :exec
-- ReapLeases returns the rows of expired claims to the pending pool.
UPDATE outbox
SET state = 'pending', claimed_at = NULL, lease_expiry = NULL
WHERE state = 'claimed' AND lease_expiry < $1;

-- name: MarkNoticePosted :exec
-- MarkNoticePosted records that a delivery's notice reply was posted, so a
-- retry skips that phase (SYSREQ-028).
UPDATE outbox
SET notice_posted = TRUE, updated_at = $2
WHERE id = $1;

-- name: MarkRootRefreshed :exec
-- MarkRootRefreshed records that a delivery's root refresh succeeded, so a
-- retry skips that phase (SYSREQ-028).
UPDATE outbox
SET root_refreshed = TRUE, updated_at = $2
WHERE id = $1;

-- name: SettleSent :exec
-- SettleSent settles a delivery that Mattermost accepted.
UPDATE outbox
SET state = 'sent', settled_at = $2, settled_reason = NULL, updated_at = $2
WHERE id = $1;

-- name: SettleSkip :exec
-- SettleSkip settles a delivery that will never be posted, with its cause
-- (SYSREQ-007).
UPDATE outbox
SET state = 'skipped', settled_reason = $2, settled_at = $3, updated_at = $3,
    claimed_at = NULL, lease_expiry = NULL
WHERE id = $1;

-- name: SettleRetry :exec
-- SettleRetry returns a failed delivery to the pool, due again at its next
-- attempt time (SYSREQ-016).
UPDATE outbox
SET state = 'pending', attempts = $2, next_attempt_at = $3, last_error = $4,
    claimed_at = NULL, lease_expiry = NULL, updated_at = $3
WHERE id = $1;

-- name: SettlePark :exec
-- SettlePark takes a delivery out of the retry rotation for good, with its
-- cause (SYSREQ-016, SYSREQ-026).
UPDATE outbox
SET state = 'parked', attempts = $2, settled_reason = $3, settled_at = $4,
    last_error = $5, claimed_at = NULL, lease_expiry = NULL, updated_at = $4
WHERE id = $1;
