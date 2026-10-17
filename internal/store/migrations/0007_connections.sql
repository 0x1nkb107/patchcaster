-- +goose Up
-- +goose StatementBegin
-- A connection carries what PatchCaster acts with on one forge or chat
-- platform (SCEN-014). The credential is sealed with the instance key
-- (SYSREQ-042). A forge connection may have none, when its hooks are
-- registered by hand. The account is whom the credential acted as when it
-- was last checked.
CREATE TABLE connections (
    id         BIGSERIAL PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('gitlab', 'mattermost')),
    name       TEXT NOT NULL UNIQUE,
    base_url   TEXT NOT NULL,
    credential BYTEA,
    account    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version    BIGINT NOT NULL DEFAULT 1
);
-- +goose StatementEnd

-- +goose StatementBegin
-- The webhook signing tokens of a forge connection, sealed (SYSREQ-044).
-- Several may be active at once, so that a token rotates without downtime.
CREATE TABLE signing_tokens (
    id            BIGSERIAL PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
    label         TEXT NOT NULL,
    token         BYTEA NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL
);
CREATE INDEX signing_tokens_connection_idx ON signing_tokens (connection_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE signing_tokens;
DROP TABLE connections;
-- +goose StatementEnd
