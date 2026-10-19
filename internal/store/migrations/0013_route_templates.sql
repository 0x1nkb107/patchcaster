-- +goose Up
-- +goose StatementBegin
-- A route may replace the built-in template of each kind of message with
-- one of its own (SYSREQ-049, owner decision 2026-10-20). A template names
-- the version of the data model it reads (ADR-038), and it goes with its
-- route.
CREATE TABLE route_templates (
    route_id     BIGINT NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    data_version INTEGER NOT NULL,
    text         TEXT NOT NULL,
    PRIMARY KEY (route_id, kind)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE route_templates;
-- +goose StatementEnd
