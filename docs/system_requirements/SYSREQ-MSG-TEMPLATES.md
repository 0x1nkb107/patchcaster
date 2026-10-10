---
id: "SYSREQ-025"
type: system_requirement
name: "Templated message rendering"
description: "Message bodies are rendered from named templates over an explicit view model."
specification: "The service SHALL render every outbound message body by applying a named message template to an explicit view model."
derives_from:
  - "SCEN-001"
  - "SCEN-002"
depends_on:
  - "SYSREQ-003"
---

# System Requirement: Templated message rendering

## Requirement Specification

> The service SHALL render every outbound message body by applying a named message template to an explicit view model.

## Rationale

Message shape is the product surface teams see all day; it will be tuned repeatedly. Separating
"which facts" (SYSREQ-003) from "how they read" keeps the Mattermost client and the renderer
independently changeable. Rendering over an explicit view model — not the raw payload — makes the
no-sensitive-content guarantee structural: there is no path from a description field to a
template, so the guarantee cannot regress by accident (owner directive on templating and
content, 2026-10-10; library choice per ADR-009).

## Acceptance Criteria

- Root and reply bodies are produced by named templates selected by message kind; the same event
  renders byte-identically across restarts (templates parsed once at startup).
- The template data is a view model exposing the five SYSREQ-003 fields plus reply context
  (acting user, action); no other payload content is reachable from a template.
- Payload-derived values enter templates only through the view model's escaped accessors, so no
  template can emit raw adversarial field text (SYSREQ-003's acceptance criteria stay intact).
- A template that fails to parse at startup prevents serving, with the failing template named in
  the exit message — same posture as SYSREQ-022.

## Verification Plan

- **Method**: Test.
- **Procedure**: Golden-text render tests per message kind, including adversarial fixtures;
  startup test with a deliberately broken embedded template asserting a non-zero exit.

## Notes

Templates are embedded in the binary for v1; operator-overridable template files are a possible
additive follow-up, not a v1 obligation *(assumed)*.
