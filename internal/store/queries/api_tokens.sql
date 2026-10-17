-- name: TokenCreate :one
-- TokenCreate stores a new token by its hash (SYSREQ-040).
INSERT INTO api_tokens (person_id, name, token_hash, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, person_id, name, created_at, expires_at, revoked_at;

-- name: TokenAuthenticate :one
-- TokenAuthenticate finds the live token with a hash: neither revoked nor
-- expired.
SELECT id, person_id, name, created_at, expires_at, revoked_at
FROM api_tokens
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > $2;

-- name: TokenGet :one
-- TokenGet reads one token of a person.
SELECT id, person_id, name, created_at, expires_at, revoked_at
FROM api_tokens
WHERE id = $1 AND person_id = $2;

-- name: TokenList :many
-- TokenList pages through a person's tokens in the order they were created.
SELECT id, person_id, name, created_at, expires_at, revoked_at
FROM api_tokens
WHERE person_id = sqlc.arg(person_id) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_size);

-- name: TokenRevoke :execrows
-- TokenRevoke revokes one live token of a person.
UPDATE api_tokens
SET revoked_at = $3
WHERE id = $1 AND person_id = $2 AND revoked_at IS NULL;

-- name: TokenRevokeAll :execrows
-- TokenRevokeAll revokes every token of a person that still works.
UPDATE api_tokens
SET revoked_at = $2
WHERE person_id = $1 AND revoked_at IS NULL AND expires_at > $2;

-- name: PersonEndSessions :execrows
-- PersonEndSessions raises a person's session epoch, which ends every
-- session that carries an earlier one.
UPDATE persons SET session_epoch = session_epoch + 1 WHERE id = $1;
