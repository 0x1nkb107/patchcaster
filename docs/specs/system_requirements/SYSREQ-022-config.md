---
id: "SYSREQ-022"
type: system_requirement
name: "Strict startup configuration"
description: "Configuration is loaded once and validated at startup."
specification: "The service SHALL load its startup and operating settings at startup from command-line flags, environment variables, and built-in defaults in that precedence, exiting with a non-zero status when configuration is incomplete or invalid."
derives_from:
  - "SCEN-008"
---

# System Requirement: Strict startup configuration

## Requirement Specification

> The service SHALL load its startup and operating settings at startup from command-line flags, environment variables, and built-in defaults in that precedence, exiting with a non-zero status when configuration is incomplete or invalid.

## Rationale

A half-configured bot that starts anyway is silently broken — deliveries fail hours later with no
cause. Failing at startup converts misconfiguration into an immediate, attributable crash loop.
Flags bound to environment variables keep Kubernetes ergonomics while preserving a self-documenting
CLI (ADR-003).

## Acceptance Criteria

- Every startup and operating setting (database URL, encryption key, interface URL, public URL,
  listen addresses, operating limits) is declared with kong and bound to a `PATCHCASTER_`-prefixed environment variable;
  the precedence is flags > environment variables > built-in defaults (owner-set, 2026-10-10).
- Any missing or malformed required value exits non-zero with a message naming the option, before
  any listener accepts a connection.

## Verification Plan

- **Method**: Test.
- **Procedure**: Kong parse table tests plus subprocess tests asserting exit codes and messages.

## Notes

Narrowed on 2026-10-18 by SYSREQ-034: connections, routes and their secrets live in the database, not in flags.