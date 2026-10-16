-- +goose Up
-- +goose StatementBegin
-- A known value sealed with the instance key. A start whose keys cannot
-- open it would misread every stored secret, so it is refused instead
-- (SYSREQ-042, ADR-035).
CREATE TABLE instance_key_check (
    id        SMALLINT PRIMARY KEY CHECK (id = 1),
    sealed    BYTEA NOT NULL,
    sealed_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE instance_key_check;
-- +goose StatementEnd
