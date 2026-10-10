---
id: "SYSREQ-022"
type: system_requirement
name: "Strict startup configuration"
description: "Configuration is loaded once and validated at startup."
specification: "The service SHALL load its complete configuration from command-line flags and environment variables at startup and exit with a non-zero status when that configuration is invalid."
derives_from:
  - "SCEN-008"
---

# System Requirement: Strict startup configuration

## Requirement Specification

> The service SHALL load its complete configuration from command-line flags and environment variables at startup and exit with a non-zero status when that configuration is invalid.

## Rationale

A half-configured bot that starts anyway is silently broken — deliveries fail hours later with no
cause. Failing at startup converts misconfiguration into an immediate, attributable crash loop.
Flags bound to environment variables keep Kubernetes ergonomics while preserving a self-documenting
CLI (ADR-003).

## Acceptance Criteria

- Every option (site URL, bot token, signing secrets, routing table path, listen addresses, state
  file path) is declared with kong and bound to a `PATCHCASTER_`-prefixed environment variable;
  an explicit flag wins over the environment *(precedence assumed)*.
- Any missing or malformed required value exits non-zero with a message naming the option, before
  any listener accepts a connection.

## Verification Plan

- **Method**: Test.
- **Procedure**: Kong parse table tests plus subprocess tests asserting exit codes and messages.

## Notes

None.
