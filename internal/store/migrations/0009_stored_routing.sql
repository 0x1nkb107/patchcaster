-- +goose Up
-- +goose StatementBegin
-- Delivery state from before the configuration lived in the database names
-- no connection and no sink. Only a development database can hold such
-- rows, because v0.2 is not released yet and refuses v0.1 databases
-- (SYSREQ-043). So they go, before the columns that now must name both.
TRUNCATE outbox, mr_thread, mr_state, seen_webhook;
-- +goose StatementEnd

-- +goose StatementBegin
-- A board belongs to a merge request of one forge connection. Its key
-- already names the connection. The column scopes push attribution to the
-- connection the push came from (SYSREQ-030).
ALTER TABLE mr_state ADD COLUMN connection_id BIGINT NOT NULL;
DROP INDEX mr_state_attribution_idx;
CREATE INDEX mr_state_attribution_idx ON mr_state (connection_id, source_project_id, source_branch) WHERE open;
-- +goose StatementEnd

-- +goose StatementBegin
-- A merge request has one thread per sink (SYSREQ-048). Removing a sink
-- removes its threads.
ALTER TABLE mr_thread DROP CONSTRAINT mr_thread_pkey;
ALTER TABLE mr_thread ADD COLUMN sink_id BIGINT NOT NULL REFERENCES sinks (id) ON DELETE CASCADE;
ALTER TABLE mr_thread ADD PRIMARY KEY (mr_ref, sink_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- A queued message names its sink and the chat connection that delivers
-- it, resolved at acceptance like its channel (SYSREQ-024). No foreign key
-- ties them: a message outlives a removed route, and a removed sink only
-- makes its messages unsendable. The holdback that keeps a merge request's
-- messages in order now applies per merge request and sink (SYSREQ-048).
ALTER TABLE outbox ADD COLUMN sink_id BIGINT NOT NULL;
ALTER TABLE outbox ADD COLUMN connection_id BIGINT NOT NULL;
DROP INDEX outbox_unsettled_mr_idx;
CREATE INDEX outbox_unsettled_mr_idx ON outbox (mr_ref, sink_id, id)
    WHERE state IN ('pending', 'claimed');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX outbox_unsettled_mr_idx;
CREATE INDEX outbox_unsettled_mr_idx ON outbox (mr_ref, id) WHERE state IN ('pending', 'claimed');
ALTER TABLE outbox DROP COLUMN connection_id, DROP COLUMN sink_id;
TRUNCATE mr_thread;
ALTER TABLE mr_thread DROP CONSTRAINT mr_thread_pkey;
ALTER TABLE mr_thread DROP COLUMN sink_id;
ALTER TABLE mr_thread ADD PRIMARY KEY (mr_ref);
DROP INDEX mr_state_attribution_idx;
CREATE INDEX mr_state_attribution_idx ON mr_state (source_project_id, source_branch) WHERE open;
ALTER TABLE mr_state DROP COLUMN connection_id;
-- +goose StatementEnd
