---
id: "SYSREQ-020"
type: system_requirement
name: "Structured correlated logs"
description: "Log records can be traced to a single webhook delivery."
specification: "The service SHALL emit structured log records in which every entry related to a webhook delivery carries its webhook identifier."
derives_from:
  - "SCEN-009"
---

# System Requirement: Structured correlated logs

## Requirement Specification

> The service SHALL emit structured log records in which every entry related to a webhook delivery carries its webhook identifier.

## Rationale

Debugging one missing or duplicate message starts from the `webhook-id` in GitLab's delivery log.
If every record carries it, one grep reconstructs the delivery's whole path; without it, the
infrastructure team guesses by timestamp.

## Acceptance Criteria

- Every log record emitted while processing a delivery carries a `webhook_id` attribute.
- Distinct records mark the decision points: verification outcome, routing outcome, queueing, and
  each delivery attempt.
- Logging goes through `log/slog` (ADR-004), with attributes rather than interpolated strings.

## Verification Plan

- **Method**: Test.
- **Procedure**: Capture handler log output in tests; assert the attribute's presence on every
  record emitted for a request and grep-equivalence per delivery.

## Notes

None.
