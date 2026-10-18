-- +goose Up
-- +goose StatementBegin
-- The hooks PatchCaster registers at a forge for the sources of a
-- connection (SYSREQ-044). A row exists for every scope the sources need,
-- and for a hook still to be removed. Until the forge registered the hook,
-- forge_hook_id is null, and the triggers and address are the ones the
-- hook is to get. The signing token is the one PatchCaster generated for
-- the hook. Neither the connection nor that token can go while the row
-- remains, so that no hook stays behind at the forge unnoticed.
CREATE TABLE forge_hooks (
    connection_id    BIGINT NOT NULL REFERENCES connections (id),
    scope_kind       TEXT NOT NULL CHECK (scope_kind IN ('instance', 'group', 'repository')),
    scope_path       TEXT NOT NULL,
    merge_requests   BOOLEAN NOT NULL,
    pushes           BOOLEAN NOT NULL,
    comments         BOOLEAN NOT NULL,
    pipelines        BOOLEAN NOT NULL,
    url              TEXT NOT NULL,
    forge_hook_id    BIGINT,
    signing_token_id BIGINT REFERENCES signing_tokens (id),
    last_error       TEXT,
    attempted_at     TIMESTAMPTZ NOT NULL,
    registered_at    TIMESTAMPTZ,
    PRIMARY KEY (connection_id, scope_kind, scope_path)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE forge_hooks;
-- +goose StatementEnd
