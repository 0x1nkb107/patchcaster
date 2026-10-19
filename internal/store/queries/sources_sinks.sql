-- name: SourceCreate :one
-- SourceCreate stores a new source of a forge connection.
INSERT INTO sources (connection_id, name, scope_kind, scope_path, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING id;

-- name: SourceGet :one
-- SourceGet reads one source with the ids of the teams it belongs to.
SELECT s.id, s.connection_id, s.name, s.scope_kind, s.scope_path, s.version,
    (SELECT COALESCE(jsonb_agg(t.team_id ORDER BY t.team_id), '[]')::jsonb FROM team_sources t WHERE t.source_id = s.id) AS teams
FROM sources s
WHERE s.id = $1;

-- name: SourceList :many
-- SourceList pages through the sources a person may see: every source for
-- an administrator, whose visible_to is zero, or those of the teams they
-- manage.
SELECT s.id, s.connection_id, s.name, s.scope_kind, s.scope_path, s.version,
    (SELECT COALESCE(jsonb_agg(t.team_id ORDER BY t.team_id), '[]')::jsonb FROM team_sources t WHERE t.source_id = s.id) AS teams
FROM sources s
WHERE s.id > sqlc.arg(after_id)
  AND (sqlc.arg(visible_to)::bigint = 0
       OR EXISTS (SELECT 1 FROM team_sources ts JOIN team_managers o ON o.team_id = ts.team_id
                  WHERE ts.source_id = s.id AND o.person_id = sqlc.arg(visible_to)::bigint))
ORDER BY s.id
LIMIT sqlc.arg(page_size);

-- name: SourceUpdate :execrows
-- SourceUpdate changes a source's name and scope at the version the caller
-- read. Its connection stays.
UPDATE sources
SET name = sqlc.arg(name), scope_kind = sqlc.arg(scope_kind), scope_path = sqlc.arg(scope_path),
    updated_at = sqlc.arg(updated_at), version = version + 1
WHERE id = sqlc.arg(id) AND version = sqlc.arg(version);

-- name: SourceDelete :execrows
-- SourceDelete removes a source at the version the caller read, with its
-- team assignments. A source a route uses cannot go.
DELETE FROM sources WHERE id = $1 AND version = $2;

-- name: SinkCreate :one
-- SinkCreate stores a new sink of a chat connection.
INSERT INTO sinks (connection_id, name, channel_id, channel_name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING id;

-- name: SinkGet :one
-- SinkGet reads one sink with the ids of the teams it belongs to.
SELECT k.id, k.connection_id, k.name, k.channel_id, k.channel_name, k.version,
    (SELECT COALESCE(jsonb_agg(t.team_id ORDER BY t.team_id), '[]')::jsonb FROM team_sinks t WHERE t.sink_id = k.id) AS teams
FROM sinks k
WHERE k.id = $1;

-- name: SinkList :many
-- SinkList pages through the sinks a person may see, as SourceList does.
SELECT k.id, k.connection_id, k.name, k.channel_id, k.channel_name, k.version,
    (SELECT COALESCE(jsonb_agg(t.team_id ORDER BY t.team_id), '[]')::jsonb FROM team_sinks t WHERE t.sink_id = k.id) AS teams
FROM sinks k
WHERE k.id > sqlc.arg(after_id)
  AND (sqlc.arg(visible_to)::bigint = 0
       OR EXISTS (SELECT 1 FROM team_sinks ts JOIN team_managers o ON o.team_id = ts.team_id
                  WHERE ts.sink_id = k.id AND o.person_id = sqlc.arg(visible_to)::bigint))
ORDER BY k.id
LIMIT sqlc.arg(page_size);

-- name: SinkUpdate :execrows
-- SinkUpdate changes a sink's name and channel at the version the caller
-- read. Its connection stays.
UPDATE sinks
SET name = sqlc.arg(name), channel_id = sqlc.arg(channel_id), channel_name = sqlc.arg(channel_name),
    updated_at = sqlc.arg(updated_at), version = version + 1
WHERE id = sqlc.arg(id) AND version = sqlc.arg(version);

-- name: SinkDelete :execrows
-- SinkDelete removes a sink at the version the caller read, with its team
-- assignments. A sink a route uses cannot go.
DELETE FROM sinks WHERE id = $1 AND version = $2;

-- name: Destinations :many
-- Destinations lists the routes an event of a forge connection reaches:
-- those of every source of the connection whose scope holds the event's
-- repository, with the sink each route delivers to and the scope of its
-- source (SYSREQ-044, SYSREQ-046). Paths match exactly, and a group holds
-- everything below it.
SELECT r.id AS route_id, r.sink_id, k.connection_id AS sink_connection_id, k.channel_id, r.muted_replies,
    s.scope_kind, s.scope_path
FROM sources s
JOIN routes r ON r.source_id = s.id
JOIN sinks k ON k.id = r.sink_id
WHERE s.connection_id = sqlc.arg(connection_id)
  AND (s.scope_kind = 'instance'
       OR (s.scope_kind = 'group' AND starts_with(sqlc.arg(repository)::text, s.scope_path || '/'))
       OR (s.scope_kind = 'repository' AND s.scope_path = sqlc.arg(repository)::text))
ORDER BY r.sink_id, r.id;

-- name: SourceScopesOfConnection :many
-- SourceScopesOfConnection lists the scopes of every source of a forge
-- connection, for the hooks they need.
SELECT id, scope_kind, scope_path
FROM sources
WHERE connection_id = $1
ORDER BY id;
