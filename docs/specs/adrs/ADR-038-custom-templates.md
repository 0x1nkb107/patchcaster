---
id: "ADR-038"
type: architecture_decision_record
name: "Custom templates over a versioned data model"
description: "Routing Managers write text/template templates against a documented data model whose versions they pin, with escaping, validation and preview built in."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-008"
---

# ADR-038: Custom templates over a versioned data model

## Context

v0.1 renders built-in templates with `text/template` over an explicit view model (ADR-009, SYSREQ-025). Once Routing Managers write templates (UC-005), the view model becomes a public contract. Templates also become an input that reaches every member of a channel.

## Decision

- `text/template` stays. Templates have no access to files, the network or the database, and they can call only a documented function set, which includes `mention`.
- The data model is documented and versioned. A template records the version it was written for. Each release keeps rendering every version still in use, or upgrades saved templates mechanically and records that in the audit log.
- Forge values arrive in the data model already escaped for Markdown (SYSREQ-051). No function returns them unescaped.
- Saving a template parses it and renders sample events for every platform. The preview shows those renderings, and size limits per platform apply at that point.
- Built-in templates ship with the binary and apply wherever a route has no template of its own.

## Consequences

- Good: Routing Managers shape their messages without being able to break delivery or the escaping.
- Bad: every data model version still in use must be maintained.
