-- name: TeamCreate :one
-- TeamCreate stores a new team without operators, sources or sinks.
INSERT INTO teams (name, created_at, updated_at)
VALUES ($1, $2, $2)
RETURNING id;

-- name: TeamGet :one
-- TeamGet reads one team with the ids of its operators, sources and sinks.
SELECT t.id, t.name, t.version,
    (SELECT COALESCE(jsonb_agg(o.person_id ORDER BY o.person_id), '[]')::jsonb FROM team_operators o WHERE o.team_id = t.id) AS operators,
    (SELECT COALESCE(jsonb_agg(s.source_id ORDER BY s.source_id), '[]')::jsonb FROM team_sources s WHERE s.team_id = t.id) AS sources,
    (SELECT COALESCE(jsonb_agg(k.sink_id ORDER BY k.sink_id), '[]')::jsonb FROM team_sinks k WHERE k.team_id = t.id) AS sinks
FROM teams t
WHERE t.id = $1;

-- name: TeamList :many
-- TeamList pages through the teams a person may see: every team for an
-- administrator, whose visible_to is zero, or the teams they operate.
SELECT t.id, t.name, t.version,
    (SELECT COALESCE(jsonb_agg(o.person_id ORDER BY o.person_id), '[]')::jsonb FROM team_operators o WHERE o.team_id = t.id) AS operators,
    (SELECT COALESCE(jsonb_agg(s.source_id ORDER BY s.source_id), '[]')::jsonb FROM team_sources s WHERE s.team_id = t.id) AS sources,
    (SELECT COALESCE(jsonb_agg(k.sink_id ORDER BY k.sink_id), '[]')::jsonb FROM team_sinks k WHERE k.team_id = t.id) AS sinks
FROM teams t
WHERE t.id > sqlc.arg(after_id)
  AND (sqlc.arg(visible_to)::bigint = 0
       OR EXISTS (SELECT 1 FROM team_operators v WHERE v.team_id = t.id AND v.person_id = sqlc.arg(visible_to)::bigint))
ORDER BY t.id
LIMIT sqlc.arg(page_size);

-- name: TeamUpdate :execrows
-- TeamUpdate renames a team at the version the caller read (ADR-043).
UPDATE teams
SET name = sqlc.arg(name), updated_at = sqlc.arg(updated_at), version = version + 1
WHERE id = sqlc.arg(id) AND version = sqlc.arg(version);

-- name: TeamDelete :execrows
-- TeamDelete removes a team at the version the caller read. Its routes must
-- go first.
DELETE FROM teams WHERE id = $1 AND version = $2;

-- name: TeamOperatorsKeep :exec
-- TeamOperatorsKeep removes the operators that are not in the JSON array of
-- person ids.
DELETE FROM team_operators
WHERE team_id = sqlc.arg(team_id)
  AND person_id NOT IN (SELECT jsonb_array_elements_text(sqlc.arg(keep)::jsonb)::bigint);

-- name: TeamOperatorsAdd :exec
-- TeamOperatorsAdd adds the operators in the JSON array of person ids.
INSERT INTO team_operators (team_id, person_id)
SELECT sqlc.arg(team_id), jsonb_array_elements_text(sqlc.arg(add)::jsonb)::bigint
ON CONFLICT DO NOTHING;

-- name: TeamSourcesKeep :exec
-- TeamSourcesKeep removes the sources that are not in the JSON array of
-- source ids. One that a route of the team uses cannot go.
DELETE FROM team_sources
WHERE team_id = sqlc.arg(team_id)
  AND source_id NOT IN (SELECT jsonb_array_elements_text(sqlc.arg(keep)::jsonb)::bigint);

-- name: TeamSourcesAdd :exec
-- TeamSourcesAdd assigns the sources in the JSON array of source ids.
INSERT INTO team_sources (team_id, source_id)
SELECT sqlc.arg(team_id), jsonb_array_elements_text(sqlc.arg(add)::jsonb)::bigint
ON CONFLICT DO NOTHING;

-- name: TeamSinksKeep :exec
-- TeamSinksKeep removes the sinks that are not in the JSON array of sink
-- ids. One that a route of the team uses cannot go.
DELETE FROM team_sinks
WHERE team_id = sqlc.arg(team_id)
  AND sink_id NOT IN (SELECT jsonb_array_elements_text(sqlc.arg(keep)::jsonb)::bigint);

-- name: TeamSinksAdd :exec
-- TeamSinksAdd assigns the sinks in the JSON array of sink ids.
INSERT INTO team_sinks (team_id, sink_id)
SELECT sqlc.arg(team_id), jsonb_array_elements_text(sqlc.arg(add)::jsonb)::bigint
ON CONFLICT DO NOTHING;

-- name: TeamOperatedBy :one
-- TeamOperatedBy reports whether a person operates a team (SYSREQ-039).
SELECT EXISTS (
    SELECT 1 FROM team_operators WHERE team_id = $1 AND person_id = $2
)::boolean AS operates;
