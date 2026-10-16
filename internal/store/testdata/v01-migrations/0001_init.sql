-- +goose Up
-- +goose StatementBegin
CREATE TABLE mr_thread (
    mr_ref       TEXT PRIMARY KEY,               -- object_attributes.id (see SYSREQ-006)
    channel_id   TEXT NOT NULL,
    root_post_id TEXT NOT NULL,
    state        TEXT NOT NULL CHECK (state IN ('open', 'closed')),
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE seen_webhook (
    webhook_id TEXT PRIMARY KEY,
    outcome    TEXT NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX seen_webhook_first_seen_idx ON seen_webhook (first_seen);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE outbox (
    id              BIGSERIAL PRIMARY KEY,
    mr_ref          TEXT NOT NULL,
    action          TEXT NOT NULL,
    dest_channel    TEXT NOT NULL,               -- resolved at ingress, immutable (SYSREQ-024)
    payload         JSONB NOT NULL,
    state           TEXT NOT NULL DEFAULT 'pending'
                    CHECK (state IN ('pending', 'claimed', 'sent', 'parked', 'skipped')),
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    claimed_at      TIMESTAMPTZ,
    lease_expiry    TIMESTAMPTZ,
    last_error      TEXT,
    settled_reason  TEXT,
    settled_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX outbox_due_idx ON outbox (state, next_attempt_at, id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE outbox;
DROP TABLE seen_webhook;
DROP TABLE mr_thread;
-- +goose StatementEnd
