---
id: "ADR-033"
type: architecture_decision_record
name: "Generic OpenID Connect login with a bootstrap link and a recovery command"
description: "People log in through the organization's OpenID Connect provider. A one-time setup link creates the first administrator, and a recovery command restores access."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-033: Generic OpenID Connect login with a bootstrap link and a recovery command

## Context

The owner chose generic OpenID Connect for login (2026-10-15). An IT department already runs an identity provider, such as Entra ID, Keycloak or Okta. A fresh instance has no provider configured yet, so the first login needs another way in.

## Considered Options

- **`github.com/coreos/go-oidc` with `golang.org/x/oauth2`**: small and widely used, but it only verifies ID tokens. Discovery wiring, PKCE, state handling and the code exchange are left to the application.
- **`github.com/zitadel/oidc`, chosen by the owner on 2026-10-15**: a relying party implementation certified by the OpenID Foundation for the basic and config profiles. It covers discovery, PKCE, state, the code exchange and token verification, and it logs through `log/slog`. Version v3.51.15 is Apache-2.0 licensed, which is compatible with AGPL-3.0.

## Decision

- Login uses the authorization code flow with PKCE through the relying party of `github.com/zitadel/oidc/v3`, built on `golang.org/x/oauth2`. PatchCaster keeps no passwords.
- The relying party keeps the flow's state and PKCE verifier in short-lived encrypted cookies. Their hash and encryption keys are derived from the instance key with HKDF from the standard library, so a login started before a restart or rollout still completes after it.
- A person is identified by issuer and subject. The first login creates the person with the role User.
- Administrators may limit login to members of named groups (owner decision 2026-10-16). Providers carry group membership in different ID token claims, so administrators name the claim as well as the allowed groups. Without a limit, anyone the provider authenticates may log in. Membership is checked at every login.
- API tokens live at most 90 days. An administrator can revoke all tokens and end all sessions of a person at once.
- Roles are assigned in PatchCaster, not taken from identity provider claims. Mapping groups to roles can come later.
- Sessions are managed by `github.com/alexedwards/scs/v2`, with their data in PostgreSQL (owner decision 2026-10-15). Version v2.9.0 is MIT licensed. The session cookie is `Secure`, `HttpOnly` and `SameSite=Lax`. Sessions end after one hour without activity and twelve hours after login at the latest. The session token is renewed at login against session fixation, and logout destroys the session.
- PatchCaster implements scs's three-method store interface itself, in its store package, over sqlc queries and a goose migration (owner decision 2026-10-16). scs's own `postgresstore` and `pgxstore` have no tagged releases, and their table would sit outside the migrations and the sqlc gate (ADR-010).
- State-changing requests pass `net/http.CrossOriginProtection` from the standard library.
- `patchcaster bootstrap` issues a single-use setup link valid for one hour while no administrator exists. Through it, the first administrator configures the provider and becomes administrator by logging in once.
  - The link carries its token in the URL fragment, which browsers never send to a server, so the token stays out of access logs. The database keeps only the token's SHA-256 hash.
  - The browser that sends the token with the provider settings must be the one that logs in: its session remembers the link, and a login in any other browser is refused while the setup is open.
  - Before the settings are stored, PatchCaster fetches the provider's discovery document and checks that it names the same issuer. The client credentials are first tested by the login.
- `patchcaster admin grant` gives the administrator role to an issuer and subject. It needs the database credentials, which already give full control, so it opens no new door.

## Consequences

- Good: the organization's identity provider keeps deciding who someone is. Offboarding there stops new logins at once. Open sessions end within twelve hours and tokens within 90 days, unless an administrator revokes them sooner.
- Good: no default password ever exists.
- Good: scs is small, handles token renewal and timeouts, and keeps sessions in the instance's own database.
- Bad: an instance cannot be used without an identity provider, even for a quick trial. A trial needs a test provider.
- Bad: the relying party package brings OpenTelemetry, `go-logr`, `go-jose`, `gorilla/securecookie` and `google/uuid` into the binary, more than `coreos/go-oidc` would.
- Bad: the library logs through the process-wide `slog` default rather than an injected logger, so PatchCaster's logger (ADR-004) must be installed as that default before the relying party is built.
