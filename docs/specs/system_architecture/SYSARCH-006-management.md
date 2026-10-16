---
id: "SYSARCH-006"
type: system_architecture
name: "Management surface"
description: "One configuration service behind the web interface and the API, with login, authorization, audit and encrypted secrets."
status: proposed
satisfies:
  - "SYSREQ-034"
  - "SYSREQ-035"
  - "SYSREQ-036"
  - "SYSREQ-037"
  - "SYSREQ-038"
  - "SYSREQ-039"
  - "SYSREQ-040"
  - "SYSREQ-041"
  - "SYSREQ-042"
  - "SYSREQ-043"
  - "SYSREQ-049"
  - "SYSREQ-052"
  - "SYSREQ-053"
  - "SYSREQ-055"
  - "SYSREQ-062"
  - "SYSREQ-064"
---

# System Architecture: Management surface

Normative once accepted: the web interface and the management API are two presentations of one configuration service, which owns authorization, auditing and secret handling.

## Technical Strategy

The configuration service is the only code that changes configuration. The API handlers are generated from the OpenAPI description (ADR-031), follow the conventions of ADR-043, and call the service, never the store directly. The web interface is a single-page application that uses only that API (ADR-032), so authorization (ADR-034) and auditing apply to both alike. Login is generic OpenID Connect with a bootstrap link and a recovery command (ADR-033). Secrets pass through a secret box that encrypts them with the instance key (ADR-035). Each instance serves one organization (ADR-029), and configuration lives in its database (ADR-030). The management surface has its own listener (SYSREQ-036).

## Static View (Structure)

Components:

- `ConfigService`: connections, teams, sources, sinks, routes, filters and templates.
- `IdentityService`: people, identity links, API tokens and the linking flow of ADR-039.
- `Authorizer`: decides every operation by role and team.
- `AuditLog`: writes an entry inside each change's transaction.
- `SecretBox`: encrypts and decrypts with the instance key.
- `Sessions`: login sessions managed by scs, with their data in PostgreSQL through PatchCaster's own store.
- `APIHandler` (generated) and the embedded single-page application, both on the management listener.
- `bootstrap` and `admin grant` commands in the binary.

Configuration model:

- **Person**: OpenID Connect issuer and subject, role, mention opt-out.
- **Team**: name, operators, and the sources and sinks assigned to it.
- **Connection**: platform kind (GitLab, Forgejo, Mattermost, Matrix), base URL, encrypted credentials, and for forges the webhook endpoint, secrets and OAuth application.
- **Source**: forge connection, scope (instance, organization or group, repository), teams, hook registration state.
- **Sink**: chat connection, channel or room, teams, deliverability state.
- **Route**: team, source, sink, reply filter, template set.
- **Template set**: root and notice templates with their data model version (ADR-038).
- **Identity link**: person, connection, account identifier.
- **API token**: owner, hash, expiry, revocation.
- **Audit entry**: actor, token, time, item, change.

## Dynamic View (Behavior)

A request authenticates by session or token, the authorizer decides it, and the service applies it. The change and its audit entry commit in one transaction. Ingest reads the routes it needs within its own work, so a committed change applies to the next accepted event. If routes are cached, a PostgreSQL notification invalidates the cache, the same mechanism as ADR-019.

## Notes

Proposed on 2026-10-15. On acceptance this extends SYSARCH-005, whose configuration then covers only startup and operating settings.
