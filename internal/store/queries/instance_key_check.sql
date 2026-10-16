-- name: KeyCheckGet :one
-- KeyCheckGet reads the sealed key check (SYSREQ-042).
SELECT sealed
FROM instance_key_check
WHERE id = 1;

-- name: KeyCheckInit :exec
-- KeyCheckInit stores the first key check. An existing one stays, so two
-- instances starting at once agree on the first.
INSERT INTO instance_key_check (id, sealed, sealed_at)
VALUES (1, $1, $2)
ON CONFLICT (id) DO NOTHING;

-- name: KeyCheckReplace :exec
-- KeyCheckReplace stores the key check sealed again, after a key rotation.
UPDATE instance_key_check
SET sealed = $1, sealed_at = $2
WHERE id = 1;
