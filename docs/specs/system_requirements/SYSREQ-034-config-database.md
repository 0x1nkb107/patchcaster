---
id: "SYSREQ-034"
type: system_requirement
name: "Configuration in the database"
description: "The database is the single source of truth for configuration. Flags and environment carry only startup and operating settings."
specification: "The service SHALL keep its complete configuration, including connections, teams, sources, sinks, routes, filters, templates, identity links and tokens, in its database as the single source of truth, and SHALL read from flags and environment only its startup and operating settings."
status: proposed
derives_from:
  - "SCEN-014"
  - "SCEN-018"
---

# System Requirement: Configuration in the database

## Requirement Specification

> The service SHALL keep its complete configuration, including connections, teams, sources, sinks, routes, filters, templates, identity links and tokens, in its database as the single source of truth, and SHALL read from flags and environment only its startup and operating settings.

## Rationale

The owner decided on 2026-10-15 that configuration lives in the database and is managed through the web interface and the API (ADR-030). Changes then take effect without a redeploy, and both surfaces edit the same state.

## Acceptance Criteria

- Startup and operating settings are the database URL, the listen addresses, the public URL, the encryption key, and operating limits such as the drain timeout, the poll interval or the log level.
- A configuration change through the web interface or the API applies to events accepted after it commits, without a restart.
- No connection, route or template can be set through a flag, an environment variable or a file, except through the one-time import of SYSREQ-043.

## Verification Plan

- **Method**: Test.
- **Procedure**: An end-to-end test changes a route through the API and asserts that the next delivery follows it, with no restart in between.

## Notes

On acceptance this narrows SYSREQ-022 to the startup and operating settings and retires the routing file of SYSREQ-009.
