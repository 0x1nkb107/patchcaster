-- name: RouteCreate :one
-- RouteCreate stores a new route. The database refuses a source or sink the
-- team does not own (SYSREQ-046).
INSERT INTO routes (team_id, source_id, sink_id, muted_replies, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING id, team_id, source_id, sink_id, muted_replies, version;

-- name: RouteGet :one
-- RouteGet reads one route.
SELECT id, team_id, source_id, sink_id, muted_replies, version
FROM routes
WHERE id = $1;

-- name: RouteList :many
-- RouteList pages through the routes a person may see: every route for an
-- administrator, whose visible_to is zero, or those of the teams they
-- manage. A team_id other than zero narrows the list to one team.
SELECT r.id, r.team_id, r.source_id, r.sink_id, r.muted_replies, r.version
FROM routes r
WHERE r.id > sqlc.arg(after_id)
  AND (sqlc.arg(team_id)::bigint = 0 OR r.team_id = sqlc.arg(team_id)::bigint)
  AND (sqlc.arg(visible_to)::bigint = 0
       OR EXISTS (SELECT 1 FROM team_managers o WHERE o.team_id = r.team_id AND o.person_id = sqlc.arg(visible_to)::bigint))
ORDER BY r.id
LIMIT sqlc.arg(page_size);

-- name: RouteUpdate :execrows
-- RouteUpdate changes a route's muted replies at the version the caller
-- read. Its team, source and sink stay.
UPDATE routes
SET muted_replies = sqlc.arg(muted_replies), updated_at = sqlc.arg(updated_at), version = version + 1
WHERE id = sqlc.arg(id) AND version = sqlc.arg(version);

-- name: RouteDelete :execrows
-- RouteDelete removes a route at the version the caller read.
DELETE FROM routes WHERE id = $1 AND version = $2;
