-- name: ConnectionHealthDelivered :exec
-- ConnectionHealthDelivered records that an authentic delivery of a forge
-- connection arrived. A connection removed meanwhile records nothing.
INSERT INTO connection_health (connection_id, last_delivery_at)
SELECT id, sqlc.arg(at) FROM connections WHERE id = sqlc.arg(connection_id)
ON CONFLICT (connection_id) DO UPDATE
SET last_delivery_at = GREATEST(connection_health.last_delivery_at, EXCLUDED.last_delivery_at);

-- name: ConnectionHealthRejected :exec
-- ConnectionHealthRejected records why a delivery of a forge connection was
-- rejected. A connection removed meanwhile records nothing.
INSERT INTO connection_health (connection_id, last_rejection_at, last_rejection)
SELECT id, sqlc.arg(at), sqlc.arg(cause) FROM connections WHERE id = sqlc.arg(connection_id)
ON CONFLICT (connection_id) DO UPDATE
SET last_rejection_at = EXCLUDED.last_rejection_at, last_rejection = EXCLUDED.last_rejection
WHERE connection_health.last_rejection_at IS NULL
   OR connection_health.last_rejection_at <= EXCLUDED.last_rejection_at;

-- name: ConnectionHealthList :many
-- ConnectionHealthList reads the delivery health of the forge connections
-- with the given ids. A connection without any delivery has no row.
SELECT connection_id, last_delivery_at, last_rejection_at, last_rejection
FROM connection_health
WHERE connection_id IN (SELECT jsonb_array_elements_text(sqlc.arg(ids)::jsonb)::bigint)
ORDER BY connection_id;
