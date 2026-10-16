-- +goose Up
-- The claim holds a row back while an older row of its merge request is
-- pending or claimed (SYSARCH-002). This partial index serves that lookup
-- from live rows only, so settled history never slows the claim.
-- +goose StatementBegin
CREATE INDEX outbox_unsettled_mr_idx ON outbox (mr_ref, id)
    WHERE state IN ('pending', 'claimed');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX outbox_unsettled_mr_idx;
-- +goose StatementEnd
