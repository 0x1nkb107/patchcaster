-- name: ConnectionNextID :one
-- ConnectionNextID reserves the id of a connection about to be created, so
-- that its credential can be sealed for that id.
SELECT nextval('connections_id_seq')::bigint AS id;

-- name: ConnectionCreate :one
-- ConnectionCreate stores a new connection under a reserved id.
INSERT INTO connections (id, kind, name, base_url, credential, account, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
RETURNING id, kind, name, base_url, credential, account, version;

-- name: ConnectionGet :one
-- ConnectionGet reads one connection.
SELECT id, kind, name, base_url, credential, account, version
FROM connections
WHERE id = $1;

-- name: ConnectionList :many
-- ConnectionList pages through connections in the order they were created.
SELECT id, kind, name, base_url, credential, account, version
FROM connections
WHERE id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_size);

-- name: ConnectionUpdate :execrows
-- ConnectionUpdate changes a connection at the version the caller read
-- (ADR-043). A null credential keeps the stored one.
UPDATE connections
SET name = sqlc.arg(name),
    base_url = sqlc.arg(base_url),
    credential = COALESCE(sqlc.narg(credential)::bytea, credential),
    account = sqlc.arg(account),
    updated_at = sqlc.arg(updated_at),
    version = version + 1
WHERE id = sqlc.arg(id) AND version = sqlc.arg(version);

-- name: ConnectionDelete :execrows
-- ConnectionDelete removes a connection at the version the caller read.
DELETE FROM connections WHERE id = $1 AND version = $2;

-- name: ConnectionCredentials :many
-- ConnectionCredentials lists the sealed credentials, for a reseal.
SELECT id, credential::bytea AS credential
FROM connections
WHERE credential IS NOT NULL
ORDER BY id;

-- name: ConnectionCredentialReplace :execrows
-- ConnectionCredentialReplace seals a credential again, but only while it
-- still holds the sealed value the caller read. A reseal changes no setting,
-- so the version stays (ADR-035).
UPDATE connections
SET credential = sqlc.arg(resealed)
WHERE id = sqlc.arg(id) AND credential = sqlc.arg(sealed);

-- name: SigningTokenNextID :one
-- SigningTokenNextID reserves the id of a signing token about to be added,
-- so that it can be sealed for that id.
SELECT nextval('signing_tokens_id_seq')::bigint AS id;

-- name: SigningTokenAdd :exec
-- SigningTokenAdd stores a sealed signing token under a reserved id.
INSERT INTO signing_tokens (id, connection_id, label, token, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: SigningTokenList :many
-- SigningTokenList pages through a connection's signing tokens, without
-- their values.
SELECT id, connection_id, label, created_at
FROM signing_tokens
WHERE connection_id = sqlc.arg(connection_id) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_size);

-- name: SigningTokenGet :one
-- SigningTokenGet reads one signing token of a connection, without its
-- value.
SELECT id, connection_id, label, created_at
FROM signing_tokens
WHERE id = $1 AND connection_id = $2;

-- name: SigningTokenRemove :execrows
-- SigningTokenRemove removes one signing token of a connection.
DELETE FROM signing_tokens WHERE id = $1 AND connection_id = $2;

-- name: SigningTokensSealed :many
-- SigningTokensSealed lists every sealed signing token, for a reseal.
SELECT id, token FROM signing_tokens ORDER BY id;

-- name: SigningTokenReplace :execrows
-- SigningTokenReplace seals a signing token again, but only while it still
-- holds the sealed value the caller read.
UPDATE signing_tokens
SET token = sqlc.arg(resealed)
WHERE id = sqlc.arg(id) AND token = sqlc.arg(sealed);
