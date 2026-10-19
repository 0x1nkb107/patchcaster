-- name: RouteTemplateGet :one
-- RouteTemplateGet reads a route's template for one kind of message.
SELECT route_id, kind, data_version, text
FROM route_templates
WHERE route_id = $1 AND kind = $2;

-- name: RouteTemplateSet :exec
-- RouteTemplateSet stores a route's template for one kind of message, in
-- place of the one it had.
INSERT INTO route_templates (route_id, kind, data_version, text)
VALUES ($1, $2, $3, $4)
ON CONFLICT (route_id, kind) DO UPDATE
SET data_version = excluded.data_version, text = excluded.text;

-- name: RouteTemplateDelete :execrows
-- RouteTemplateDelete removes a route's template for one kind of message.
DELETE FROM route_templates WHERE route_id = $1 AND kind = $2;
