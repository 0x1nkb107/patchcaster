-- name: SessionFind :one
-- SessionFind reads an unexpired session for the scs store (ADR-033).
SELECT data FROM sessions WHERE token = $1 AND expiry > $2;

-- name: SessionCommit :exec
-- SessionCommit stores a session for the scs store.
INSERT INTO sessions (token, data, expiry)
VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE SET data = EXCLUDED.data, expiry = EXCLUDED.expiry;

-- name: SessionDelete :exec
-- SessionDelete ends a session.
DELETE FROM sessions WHERE token = $1;

-- name: SessionDeleteExpired :exec
-- SessionDeleteExpired removes sessions past their expiry.
DELETE FROM sessions WHERE expiry <= $1;
