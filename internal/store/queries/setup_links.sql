-- name: SetupLinkCreate :exec
-- SetupLinkCreate stores the hash of a new setup link (SYSREQ-038).
INSERT INTO setup_links (token_hash, created_at, expires_at)
VALUES ($1, $2, $3);

-- name: SetupLinkValid :one
-- SetupLinkValid reports whether a setup link is unused and unexpired.
SELECT EXISTS (
    SELECT 1 FROM setup_links
    WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
)::boolean AS valid;

-- name: SetupLinkUse :execrows
-- SetupLinkUse spends a setup link, once.
UPDATE setup_links
SET used_at = $2
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2;
