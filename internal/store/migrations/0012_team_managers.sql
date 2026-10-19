-- +goose Up
-- +goose StatementBegin
-- The people who route a team's sources are its managers, shown as
-- Routing Managers (owner decision 2026-10-19). Constraints and indexes
-- keep their names on a table rename, so they follow one by one.
ALTER TABLE team_operators RENAME TO team_managers;
ALTER TABLE team_managers RENAME CONSTRAINT team_operators_pkey TO team_managers_pkey;
ALTER TABLE team_managers RENAME CONSTRAINT team_operators_team_id_fkey TO team_managers_team_id_fkey;
ALTER TABLE team_managers RENAME CONSTRAINT team_operators_person_id_fkey TO team_managers_person_id_fkey;
ALTER INDEX team_operators_person_idx RENAME TO team_managers_person_idx;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER INDEX team_managers_person_idx RENAME TO team_operators_person_idx;
ALTER TABLE team_managers RENAME CONSTRAINT team_managers_person_id_fkey TO team_operators_person_id_fkey;
ALTER TABLE team_managers RENAME CONSTRAINT team_managers_team_id_fkey TO team_operators_team_id_fkey;
ALTER TABLE team_managers RENAME CONSTRAINT team_managers_pkey TO team_operators_pkey;
ALTER TABLE team_managers RENAME TO team_operators;
-- +goose StatementEnd
