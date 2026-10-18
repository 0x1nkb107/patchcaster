-- name: SinkHealthSucceeded :exec
-- SinkHealthSucceeded records that the platform accepted a message for a
-- sink. A sink removed meanwhile records nothing, and an earlier success
-- that arrives late leaves a later one standing.
INSERT INTO sink_health (sink_id, last_success_at)
SELECT id, sqlc.arg(at) FROM sinks WHERE id = sqlc.arg(sink_id)
ON CONFLICT (sink_id) DO UPDATE
SET last_success_at = GREATEST(sink_health.last_success_at, EXCLUDED.last_success_at);

-- name: SinkHealthFailed :exec
-- SinkHealthFailed records why an attempt at a sink's platform failed. A
-- sink removed meanwhile records nothing, and an earlier failure that
-- arrives late leaves a later one standing.
INSERT INTO sink_health (sink_id, last_failure_at, last_error)
SELECT id, sqlc.arg(at), sqlc.arg(error) FROM sinks WHERE id = sqlc.arg(sink_id)
ON CONFLICT (sink_id) DO UPDATE
SET last_failure_at = EXCLUDED.last_failure_at, last_error = EXCLUDED.last_error
WHERE sink_health.last_failure_at IS NULL OR sink_health.last_failure_at <= EXCLUDED.last_failure_at;

-- name: SinkHealthList :many
-- SinkHealthList reads the health of the sinks with the given ids: their
-- waiting messages, the oldest of them, and their last success and
-- failure.
SELECT k.id, k.connection_id, h.last_success_at, h.last_failure_at, h.last_error,
    count(o.id) AS waiting, min(o.created_at) AS oldest_waiting
FROM sinks k
LEFT JOIN sink_health h ON h.sink_id = k.id
LEFT JOIN outbox o ON o.sink_id = k.id AND o.state IN ('pending', 'claimed')
WHERE k.id IN (SELECT jsonb_array_elements_text(sqlc.arg(ids)::jsonb)::bigint)
GROUP BY k.id, h.sink_id
ORDER BY k.id;

-- name: SinkHealthOfConnections :many
-- SinkHealthOfConnections reads the health of every sink of the chat
-- connections with the given ids, as SinkHealthList does.
SELECT k.id, k.connection_id, h.last_success_at, h.last_failure_at, h.last_error,
    count(o.id) AS waiting, min(o.created_at) AS oldest_waiting
FROM sinks k
LEFT JOIN sink_health h ON h.sink_id = k.id
LEFT JOIN outbox o ON o.sink_id = k.id AND o.state IN ('pending', 'claimed')
WHERE k.connection_id IN (SELECT jsonb_array_elements_text(sqlc.arg(connection_ids)::jsonb)::bigint)
GROUP BY k.id, h.sink_id
ORDER BY k.id;

-- name: SinkQueues :many
-- SinkQueues counts the waiting messages of every sink and finds the
-- oldest, for the waiting gauges.
SELECT k.id, k.connection_id, count(o.id) AS waiting, min(o.created_at) AS oldest_waiting
FROM sinks k
LEFT JOIN outbox o ON o.sink_id = k.id AND o.state IN ('pending', 'claimed')
GROUP BY k.id
ORDER BY k.id;
