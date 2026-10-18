-- +goose Up
-- +goose StatementBegin
-- How each sink delivers (SYSREQ-064). The worker writes it after every
-- attempt at the platform. It is state, not configuration, so it lives
-- apart from the sink and never changes the sink's version.
CREATE TABLE sink_health (
    sink_id         BIGINT PRIMARY KEY REFERENCES sinks (id) ON DELETE CASCADE,
    last_success_at TIMESTAMPTZ,
    last_failure_at TIMESTAMPTZ,
    last_error      TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
-- How each forge connection's deliveries arrive (SYSREQ-064). The webhook
-- endpoint writes it after every delivery whose connection it knows.
CREATE TABLE connection_health (
    connection_id     BIGINT PRIMARY KEY REFERENCES connections (id) ON DELETE CASCADE,
    last_delivery_at  TIMESTAMPTZ,
    last_rejection_at TIMESTAMPTZ,
    last_rejection    TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
-- The waiting messages of a sink and the oldest of them, for its health
-- and the waiting gauges.
CREATE INDEX outbox_waiting_sink_idx ON outbox (sink_id, created_at)
    WHERE state IN ('pending', 'claimed');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX outbox_waiting_sink_idx;
DROP TABLE connection_health;
DROP TABLE sink_health;
-- +goose StatementEnd
