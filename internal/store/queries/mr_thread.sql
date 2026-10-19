-- name: ThreadGet :one
-- ThreadGet reads the thread mapping of a merge request in one sink
-- (SYSREQ-006, SYSREQ-048).
SELECT mr_ref, channel_id, root_post_id, state, route_id
FROM mr_thread
WHERE mr_ref = $1 AND sink_id = $2;

-- name: ThreadOpen :exec
-- ThreadOpen maps a merge request to its root post in one sink, with the
-- route whose templates wrote the root. An existing mapping stays as it is,
-- so the first root wins.
INSERT INTO mr_thread (mr_ref, sink_id, channel_id, root_post_id, route_id, state, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 'open', $6, $6)
ON CONFLICT (mr_ref, sink_id) DO NOTHING;

-- name: ThreadClose :exec
-- ThreadClose closes a mapping in one sink, so later deliveries stop
-- replying to it (SYSREQ-026).
UPDATE mr_thread
SET state = 'closed', updated_at = $3
WHERE mr_ref = $1 AND sink_id = $2;
