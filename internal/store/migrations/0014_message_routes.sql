-- +goose Up
-- +goose StatementBegin
-- A queued message names the route whose templates write it: of the routes
-- that reach its sink, the one with the narrowest source (owner decision
-- 2026-10-20). A thread keeps the route of its root, so that its messages
-- keep one look. Like the sink, the route has no foreign key: a message
-- outlives a removed route, and a removed route only means the built-in
-- templates. Route ids are never reused, so a removed route stays removed.
ALTER TABLE outbox ADD COLUMN route_id BIGINT;
ALTER TABLE mr_thread ADD COLUMN route_id BIGINT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE mr_thread DROP COLUMN route_id;
ALTER TABLE outbox DROP COLUMN route_id;
-- +goose StatementEnd
