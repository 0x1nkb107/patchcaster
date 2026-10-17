-- +goose Up
-- +goose StatementBegin
-- A person is who the identity provider says they are: an issuer and a
-- subject (SYSREQ-037). Roles are assigned here, not taken from the
-- provider (ADR-033).
CREATE TABLE persons (
    id               BIGSERIAL PRIMARY KEY,
    issuer           TEXT NOT NULL,
    subject          TEXT NOT NULL,
    display_name     TEXT NOT NULL DEFAULT '',
    email            TEXT NOT NULL DEFAULT '',
    is_administrator BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL,
    last_login_at    TIMESTAMPTZ,
    version          BIGINT NOT NULL DEFAULT 1,
    UNIQUE (issuer, subject)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- The OpenID Connect provider an administrator configured. One row: an
-- instance has one provider. The client secret is sealed with the instance
-- key (SYSREQ-042).
CREATE TABLE login_settings (
    id             SMALLINT PRIMARY KEY CHECK (id = 1),
    issuer         TEXT NOT NULL,
    client_id      TEXT NOT NULL,
    client_secret  BYTEA NOT NULL,
    groups_claim   TEXT NOT NULL DEFAULT '',
    allowed_groups JSONB NOT NULL DEFAULT '[]', -- a JSON array of group names
    updated_at     TIMESTAMPTZ NOT NULL,
    version        BIGINT NOT NULL DEFAULT 1
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Single-use setup links of the bootstrap command, kept only as hashes
-- (SYSREQ-038).
CREATE TABLE setup_links (
    token_hash BYTEA PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Login sessions in the shape the scs session manager stores (ADR-033).
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);
-- +goose StatementEnd

-- +goose StatementBegin
-- Every configuration change and every administrator's login, written in the
-- change's own transaction and never edited (SYSREQ-041). The actor label
-- keeps who acted readable after the person is gone.
CREATE TABLE audit_log (
    id              BIGSERIAL PRIMARY KEY,
    at              TIMESTAMPTZ NOT NULL,
    actor_person_id BIGINT REFERENCES persons (id) ON DELETE SET NULL,
    actor_label     TEXT NOT NULL,
    action          TEXT NOT NULL,
    target_kind     TEXT NOT NULL,
    target_id       TEXT NOT NULL DEFAULT '',
    before          JSONB NOT NULL DEFAULT 'null', -- null when there was nothing before
    after           JSONB NOT NULL DEFAULT 'null'  -- null when nothing is left after
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE audit_log;
DROP TABLE sessions;
DROP TABLE setup_links;
DROP TABLE login_settings;
DROP TABLE persons;
-- +goose StatementEnd
