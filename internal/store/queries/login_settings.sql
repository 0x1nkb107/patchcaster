-- name: LoginSettingsGet :one
-- LoginSettingsGet reads the configured OpenID Connect provider.
SELECT issuer, client_id, client_secret, groups_claim, allowed_groups, version
FROM login_settings
WHERE id = 1;

-- name: LoginSettingsPut :exec
-- LoginSettingsPut stores the provider during the setup, replacing what an
-- earlier setup attempt stored.
INSERT INTO login_settings (id, issuer, client_id, client_secret, groups_claim, allowed_groups, updated_at)
VALUES (1, $1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE
SET issuer = EXCLUDED.issuer,
    client_id = EXCLUDED.client_id,
    client_secret = EXCLUDED.client_secret,
    groups_claim = EXCLUDED.groups_claim,
    allowed_groups = EXCLUDED.allowed_groups,
    updated_at = EXCLUDED.updated_at,
    version = login_settings.version + 1;

-- name: LoginSettingsUpdate :execrows
-- LoginSettingsUpdate changes the provider at the version the caller read
-- (ADR-043). A null client secret keeps the stored one.
UPDATE login_settings
SET issuer = sqlc.arg(issuer),
    client_id = sqlc.arg(client_id),
    client_secret = COALESCE(sqlc.narg(client_secret)::bytea, client_secret),
    groups_claim = sqlc.arg(groups_claim),
    allowed_groups = sqlc.arg(allowed_groups),
    updated_at = sqlc.arg(updated_at),
    version = version + 1
WHERE id = 1 AND version = sqlc.arg(version);

-- name: LoginClientSecretReplace :execrows
-- LoginClientSecretReplace seals the client secret again, but only while it
-- still holds the sealed value the caller read. A reseal changes no setting,
-- so the version stays (ADR-035).
UPDATE login_settings
SET client_secret = sqlc.arg(resealed)
WHERE id = 1 AND client_secret = sqlc.arg(sealed);
