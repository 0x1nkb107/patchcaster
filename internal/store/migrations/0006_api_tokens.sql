-- +goose Up
-- +goose StatementBegin
-- Personal API tokens (SYSREQ-040). Only a hash of each token is kept, so
-- the database alone authenticates nobody. A revoked token stays listed,
-- so that its owner sees what it was.
CREATE TABLE api_tokens (
    id         BIGSERIAL PRIMARY KEY,
    person_id  BIGINT NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX api_tokens_person_idx ON api_tokens (person_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- A session counts only while it carries its person's current session
-- epoch. Raising the epoch ends every session of the person at once
-- (SYSREQ-040).
ALTER TABLE persons ADD COLUMN session_epoch BIGINT NOT NULL DEFAULT 1;
-- +goose StatementEnd

-- +goose StatementBegin
-- A change made with a token names the token as well as its owner. The name
-- keeps the entry readable after the token is gone (SYSREQ-041).
ALTER TABLE audit_log
    ADD COLUMN actor_token_id BIGINT REFERENCES api_tokens (id) ON DELETE SET NULL,
    ADD COLUMN actor_token_name TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE audit_log DROP COLUMN actor_token_name, DROP COLUMN actor_token_id;
ALTER TABLE persons DROP COLUMN session_epoch;
DROP TABLE api_tokens;
-- +goose StatementEnd
