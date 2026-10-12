-- name: Ping :one
-- Ping proves that the database answers a query, for the readiness probe
-- (SYSREQ-018).
SELECT 1;
