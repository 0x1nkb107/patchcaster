-- name: AuditAppend :exec
-- AuditAppend records one change, inside the change's own transaction
-- (SYSREQ-041).
INSERT INTO audit_log (at, actor_person_id, actor_label, actor_token_id, actor_token_name, action, target_kind, target_id, before, after)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: AuditList :many
-- AuditList pages through the record, newest first.
SELECT id, at, actor_person_id, actor_label, actor_token_id, actor_token_name, action, target_kind, target_id, before, after
FROM audit_log
WHERE sqlc.arg(before_id)::bigint = 0 OR id < sqlc.arg(before_id)::bigint
ORDER BY id DESC
LIMIT sqlc.arg(page_size);
