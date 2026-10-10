---
id: "SYSREQ-021"
type: system_requirement
name: "No secrets in logs"
description: "Tokens and signing secrets stay out of observable output."
specification: "The service MUST NOT write the Mattermost bot token or any webhook signing secret into logs, error messages, or metrics."
derives_from:
  - "SCEN-009"
---

# System Requirement: No secrets in logs

## Requirement Specification

> The service MUST NOT write the Mattermost bot token or any webhook signing secret into logs, error messages, or metrics.

## Rationale

Logs leak into tickets and pastes. A bot token in a log means anyone with it posts as the bot; a
signing secret means anyone can forge deliveries past SYSREQ-012. The cheapest guarantee is that
they never enter observable output at all.

## Acceptance Criteria

- With sentinel secret values configured, no log record, error string, HTTP response body, or
  metric label contains them — on success paths and on authentication, delivery, and startup
  failure paths.
- Startup never logs the loaded configuration values wholesale.

## Verification Plan

- **Method**: Test.
- **Procedure**: Sentinel-value scan across captured logs and error output for happy, failing,
  and misconfigured runs.

## Notes

Error wrapping must not interpolate secret-bearing values into messages (ADR-005).
