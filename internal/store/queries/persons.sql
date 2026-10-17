-- name: PersonUpsertLogin :one
-- PersonUpsertLogin records a login: it creates the person at their first
-- login, with the role User, and refreshes their name and email afterwards
-- (SYSREQ-037).
INSERT INTO persons (issuer, subject, display_name, email, created_at, last_login_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (issuer, subject) DO UPDATE
SET display_name = EXCLUDED.display_name,
    email = EXCLUDED.email,
    last_login_at = EXCLUDED.last_login_at
RETURNING id, issuer, subject, display_name, email, is_administrator, version;

-- name: PersonGet :one
-- PersonGet reads one person.
SELECT id, issuer, subject, display_name, email, is_administrator, version
FROM persons
WHERE id = $1;

-- name: PersonList :many
-- PersonList pages through people in the order they first logged in.
SELECT id, issuer, subject, display_name, email, is_administrator, version
FROM persons
WHERE id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_size);

-- name: PersonSetAdministrator :execrows
-- PersonSetAdministrator changes a person's role, but only at the version
-- the caller read, so that no change is lost (ADR-043).
UPDATE persons
SET is_administrator = $2, version = version + 1
WHERE id = $1 AND version = $3;

-- name: PersonGrantAdministrator :one
-- PersonGrantAdministrator makes a person an administrator, creating the
-- person when they never logged in. It serves the recovery command
-- (SYSREQ-038).
INSERT INTO persons (issuer, subject, is_administrator, created_at)
VALUES ($1, $2, TRUE, $3)
ON CONFLICT (issuer, subject) DO UPDATE
SET is_administrator = TRUE, version = persons.version + 1
RETURNING id;

-- name: AdministratorCount :one
-- AdministratorCount counts the administrators, so that bootstrap knows
-- whether it may still run and the last administrator cannot be demoted.
SELECT count(*) FROM persons WHERE is_administrator;
