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
  - The functions are `mention`, for `here` and `channel` (owner decision 2026-10-20), `code` and `plural`, plus the comparisons and the logic of `text/template`. Its other functions could cut forge values apart or print them in another form, so validation refuses them.
  - Validation also refuses `range`, `define`, `block` and `template`. Without loops, the size of a message depends on its values only.
- A route may replace the template of each kind of message on its own: the root and each kind of notice. A kind without a template of its own uses the built-in one (owner decision 2026-10-20).
- The data model is documented and versioned. A template records the version it was written for. Each release keeps rendering every version still in use, or upgrades saved templates mechanically and records that in the audit log.
  - Version 1 gives every message the merge request and its status, and a notice the event as well. Pipeline states use forge-neutral words (owner decision 2026-10-20). The user guide on templates documents its fields.
- Forge values arrive in the data model already escaped for Markdown (SYSREQ-051). No function returns them unescaped.
- Saving a template parses it and renders sample events for every platform. The preview shows those renderings, and size limits per platform apply at that point.
  - The samples cover each pipeline state, open and closed merge requests, and the longest values a forge sends, made of the character whose escaping grows most. A refused template names the line and the cause.
- Built-in templates ship with the binary and apply wherever a route has no template of its own.
- When several routes reach one sink, the route with the narrowest source writes the sink's message, and the thread keeps that route while it exists (owner decision 2026-10-20). Without it, the route the message was queued with writes it.
- A stored template that fails at delivery gives way to the built-in one, with a warning and a count, so that the message still goes out (owner decision 2026-10-20).

## Consequences

- Good: Routing Managers shape their messages without being able to break delivery or the escaping.
- Bad: every data model version still in use must be maintained.
