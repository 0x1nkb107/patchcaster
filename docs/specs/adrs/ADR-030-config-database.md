---
id: "ADR-030"
type: architecture_decision_record
name: "Configuration lives in the database"
description: "Connections, teams, sources, sinks, routes and templates live in the database and change through the web interface and the API. Flags and environment keep only startup and operating settings."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
supersedes:
  - "ADR-013"
---

# ADR-030: Configuration lives in the database

## Context

v0.1 is configured by flags, environment variables and a routing file (UC-002, SYSREQ-009, SYSREQ-022, ADR-012, ADR-013). Changing a route means a redeploy, and teams cannot change their own routing. On 2026-10-15 the owner decided that the database is the single source of truth, accessed through the web interface or the API, with published OpenAPI specs.

## Considered Options

- **Flags, environment and files, as today**: declarative and GitOps-friendly, but every change is a redeploy, and per-team self-service is impossible.
- **The database, with file-managed entries shown read-only**: two sources of truth, and a precedence rule every reader must learn.
- **The database only, chosen**: one source of truth. GitOps returns as clients of the API.

## Decision

- The database holds connections, teams, sources, sinks, routes, filters, templates, identity links and tokens.
- Flags and environment keep the startup and operating settings: the database URL, the listen addresses, the public base URL, the interface URL, the encryption key, and the operating limits such as the drain timeout, the poll interval and the log level.
- A committed change applies to the next accepted event, without a restart.
- Declarative configuration returns as API clients, for example a later `patchcaster apply` command or a Terraform provider generated from the OpenAPI description (ADR-031).
- v0.2 is an incompatible upgrade (owner decision 2026-10-16). v0.1 deployments start with an empty database, and their threads are abandoned (SYSREQ-043).

## Consequences

- Good: Routing Managers change routing themselves, and changes need no redeploy.
- Bad: the configuration is no longer visible in a Git diff by default. The audit log (SYSREQ-041) and API clients take over that role.
- Bad: backups of the database now also back up the configuration, so they must be treated like it.
- With its acceptance on 2026-10-18, ADR-012 validates only the startup settings, ADR-013 is superseded with the routing file, and UC-005 replaces UC-002.
