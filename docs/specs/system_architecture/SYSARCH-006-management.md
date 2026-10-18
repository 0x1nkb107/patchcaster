---
id: "SYSARCH-006"
type: system_architecture
name: "Management surface"
description: "One configuration service behind the web interface and the API, with login, authorization, audit and encrypted secrets."
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

Accepted in part on 2026-10-18, with increment 1 (SOL-001). Still planned, and not normative until their increment is accepted: operator templates (ADR-038), with increment 2, and identity links (ADR-039), with increment 5.

Normative: the web interface and the management API are two presentations of one configuration service, which owns authorization, auditing and secret handling.

## Technical Strategy

The configuration service is the only code that changes configuration. The API handlers are generated from the OpenAPI description (ADR-031), follow the conventions of ADR-043, and call the service, never the store directly. The web interface is a single-page application that uses only that API (ADR-032), so authorization (ADR-034) and auditing apply to both alike. Login is generic OpenID Connect with a bootstrap link and a recovery command (ADR-033). Secrets pass through a secret box that encrypts them with the instance key (ADR-035). Each instance serves one organization (ADR-029), and configuration lives in its database (ADR-030). The management surface has its own listener (SYSREQ-036). The health of connections and sinks (SYSREQ-064) is state, not configuration: the webhook endpoint and the delivery worker write it into tables of its own, and the API shows it as a read-only part of each connection and sink, outside their versions.

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

Accepted in part on 2026-10-18. It extends SYSARCH-005, whose configuration covers only startup and operating settings.
