-- name: ForgeHookList :many
-- ForgeHookList reads the hooks of a connection, registered or not.
SELECT connection_id, scope_kind, scope_path, merge_requests, pushes, comments, pipelines,
    url, forge_hook_id, signing_token_id, last_error, attempted_at, registered_at
FROM forge_hooks
WHERE connection_id = $1
ORDER BY scope_kind, scope_path;

-- name: ForgeHookRegistered :exec
-- ForgeHookRegistered records a hook the forge registered, and clears an
-- earlier failure.
INSERT INTO forge_hooks (connection_id, scope_kind, scope_path, merge_requests, pushes, comments,
    pipelines, url, forge_hook_id, signing_token_id, last_error, attempted_at, registered_at)
VALUES (sqlc.arg(connection_id), sqlc.arg(scope_kind), sqlc.arg(scope_path), sqlc.arg(merge_requests),
    sqlc.arg(pushes), sqlc.arg(comments), sqlc.arg(pipelines), sqlc.arg(url), sqlc.arg(forge_hook_id),
    sqlc.arg(signing_token_id), NULL, sqlc.arg(at), sqlc.arg(at))
ON CONFLICT (connection_id, scope_kind, scope_path) DO UPDATE
SET merge_requests = EXCLUDED.merge_requests, pushes = EXCLUDED.pushes, comments = EXCLUDED.comments,
    pipelines = EXCLUDED.pipelines, url = EXCLUDED.url, forge_hook_id = EXCLUDED.forge_hook_id,
    signing_token_id = EXCLUDED.signing_token_id, last_error = NULL,
    attempted_at = EXCLUDED.attempted_at, registered_at = EXCLUDED.registered_at;

-- name: ForgeHookChanged :execrows
-- ForgeHookChanged records the new triggers and address of a registered
-- hook, and clears an earlier failure.
UPDATE forge_hooks
SET merge_requests = sqlc.arg(merge_requests), pushes = sqlc.arg(pushes), comments = sqlc.arg(comments),
    pipelines = sqlc.arg(pipelines), url = sqlc.arg(url), last_error = NULL, attempted_at = sqlc.arg(at)
WHERE connection_id = sqlc.arg(connection_id) AND scope_kind = sqlc.arg(scope_kind)
  AND scope_path = sqlc.arg(scope_path) AND forge_hook_id IS NOT NULL;

-- name: ForgeHookFailed :exec
-- ForgeHookFailed records why registering, changing or removing a hook
-- failed. A scope without a row gets one with the hook it is to get, and a
-- registered hook keeps what the forge holds.
INSERT INTO forge_hooks (connection_id, scope_kind, scope_path, merge_requests, pushes, comments,
    pipelines, url, last_error, attempted_at)
VALUES (sqlc.arg(connection_id), sqlc.arg(scope_kind), sqlc.arg(scope_path), sqlc.arg(merge_requests),
    sqlc.arg(pushes), sqlc.arg(comments), sqlc.arg(pipelines), sqlc.arg(url), sqlc.arg(error), sqlc.arg(at))
ON CONFLICT (connection_id, scope_kind, scope_path) DO UPDATE
SET last_error = EXCLUDED.last_error, attempted_at = EXCLUDED.attempted_at;

-- name: ForgeHookDelete :one
-- ForgeHookDelete removes the row of a hook and returns its signing token,
-- if it has one.
DELETE FROM forge_hooks
WHERE connection_id = $1 AND scope_kind = $2 AND scope_path = $3
RETURNING signing_token_id;

-- name: ForgeHookLock :one
-- ForgeHookLock takes the lock on registering the hooks of a connection
-- until the transaction ends, unless another session holds it.
SELECT pg_try_advisory_xact_lock(sqlc.arg(class)::int, sqlc.arg(connection_id)::int) AS locked;
