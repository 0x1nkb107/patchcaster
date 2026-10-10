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
RETURNING id, mr_ref, action, dest_channel, payload, attempts,
          notice_posted, root_refreshed;

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

-- Live-state board (SYSREQ-027..030). mr_state rows are created by the first
-- delivery that names the merge request, including note/pipeline events whose
-- merge request event has not arrived yet.

-- name: StateEnsure :exec
INSERT INTO mr_state (mr_ref, updated_at)
VALUES ($1, $2)
ON CONFLICT (mr_ref) DO NOTHING;

-- name: StateGetForUpdate :one
SELECT mr_ref, title, author, url, iid, source_branch, target_branch, project_path, source_project_id, open,
       pipeline_status, comment_count, edit_count, reviewers_done,
       reviewers_required, last_push_count
FROM mr_state WHERE mr_ref = $1
FOR UPDATE;

-- name: StateGet :one
SELECT mr_ref, title, author, url, iid, source_branch, target_branch, project_path, source_project_id, open,
       pipeline_status, comment_count, edit_count, reviewers_done,
       reviewers_required, last_push_count
FROM mr_state WHERE mr_ref = $1;

-- name: StateApplyMR :exec
UPDATE mr_state
SET title = $2, author = $3, url = $4, iid = $5, source_branch = $6, target_branch = $7,
    source_project_id = $8, open = $9, reviewers_done = $10,
    reviewers_required = $11, edit_count = edit_count + $12, updated_at = $13, project_path = $14
WHERE mr_ref = $1;

-- name: StateSetPipeline :execrows
UPDATE mr_state
SET pipeline_status = $2, updated_at = $3
WHERE mr_ref = $1 AND pipeline_status <> $2;

-- name: StateIncrComment :execrows
UPDATE mr_state
SET comment_count = comment_count + 1, updated_at = $2
WHERE mr_ref = $1;

-- name: StateAttributePush :execrows
UPDATE mr_state
SET last_push_count = $3, updated_at = $4
WHERE open AND source_project_id = $1 AND source_branch = $2;

-- name: StateClose :exec
UPDATE mr_state SET open = FALSE, updated_at = $2 WHERE mr_ref = $1;

-- name: MarkNoticePosted :exec
UPDATE outbox SET notice_posted = TRUE, updated_at = $2 WHERE id = $1;

-- name: MarkRootRefreshed :exec
UPDATE outbox SET root_refreshed = TRUE, updated_at = $2 WHERE id = $1;
