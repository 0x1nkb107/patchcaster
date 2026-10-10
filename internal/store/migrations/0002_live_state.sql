-- +goose Up
-- mr_state: the per-merge-request board (SYSREQ-027, SYSARCH-002). Rows may
-- exist before the thread is announced (linked note/pipeline events can
-- arrive first) and survive thread closure; header fields hold the latest
-- merge request facts so any delivery can re-render the root without a
-- GitLab read.
-- +goose StatementBegin
CREATE TABLE mr_state (
    mr_ref             TEXT PRIMARY KEY,            -- object_attributes.id (see SYSREQ-006)
    title              TEXT NOT NULL DEFAULT '',
    author             TEXT NOT NULL DEFAULT '',
    url                TEXT NOT NULL DEFAULT '',
    iid                BIGINT NOT NULL DEFAULT 0,
    source_branch      TEXT NOT NULL DEFAULT '',    -- push attribution key (SYSREQ-030)
    target_branch      TEXT NOT NULL DEFAULT '',    -- root header line (SYSREQ-003)
    project_path       TEXT NOT NULL DEFAULT '',    -- root header line (SYSREQ-003)
    source_project_id  BIGINT NOT NULL DEFAULT 0,
    open               BOOLEAN NOT NULL DEFAULT TRUE,
    pipeline_status    TEXT NOT NULL DEFAULT '',    -- '' = never observed: no segment (SYSREQ-027)
    comment_count      INT NOT NULL DEFAULT 0,
    edit_count         INT NOT NULL DEFAULT 0,
    reviewers_done     INT NOT NULL DEFAULT 0,
    reviewers_required INT NOT NULL DEFAULT 0,
    last_push_count    INT NOT NULL DEFAULT 0,      -- 0 = unattributed (SYSREQ-030)
    updated_at         TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX mr_state_attribution_idx ON mr_state (source_project_id, source_branch) WHERE open;
-- +goose StatementEnd

-- Outbox phase flags: which Mattermost operations of a two-phase delivery
-- (notice reply, root refresh) already succeeded, so retries skip completed
-- work (SYSREQ-028, ADR-014).
-- +goose StatementBegin
ALTER TABLE outbox ADD COLUMN notice_posted BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE outbox ADD COLUMN root_refreshed BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE outbox DROP COLUMN root_refreshed;
ALTER TABLE outbox DROP COLUMN notice_posted;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE mr_state;
-- +goose StatementEnd
