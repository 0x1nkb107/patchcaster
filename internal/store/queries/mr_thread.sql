-- name: ThreadGet :one
-- ThreadGet reads the thread mapping of a merge request (SYSREQ-006).
SELECT mr_ref, channel_id, root_post_id, state
FROM mr_thread
WHERE mr_ref = $1;

-- name: ThreadOpen :exec
-- ThreadOpen maps a merge request to its root post. An existing mapping
-- stays as it is, so the first root wins.
INSERT INTO mr_thread (mr_ref, channel_id, root_post_id, state, created_at, updated_at)
VALUES ($1, $2, $3, 'open', $4, $4)
ON CONFLICT (mr_ref) DO NOTHING;

-- name: ThreadClose :exec
-- ThreadClose closes a mapping, so later deliveries stop replying to it
-- (SYSREQ-026).
UPDATE mr_thread
SET state = 'closed', updated_at = $2
WHERE mr_ref = $1;
