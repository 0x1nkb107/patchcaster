---
id: "SYSREQ-049"
type: system_requirement
name: "Custom templates"
description: "Routing Managers replace a route's built-in templates with their own, over a documented and versioned data model, after validation and a preview."
specification: "The service SHALL let Routing Managers replace the built-in root and notice templates of a route with their own Markdown templates over a documented and versioned data model, SHALL refuse a template that does not parse or does not render every sample event, and SHALL preview a template for each sink platform before it is saved."
status: proposed
derives_from:
  - "SCEN-021"
depends_on:
  - "SYSREQ-025"
---

# System Requirement: Custom templates

## Requirement Specification

> The service SHALL let Routing Managers replace the built-in root and notice templates of a route with their own Markdown templates over a documented and versioned data model, SHALL refuse a template that does not parse or does not render every sample event, and SHALL preview a template for each sink platform before it is saved.

## Rationale

The owner wants teams to decide how messages look (2026-10-15). Once Routing Managers write templates, the data model they write against becomes a public contract, so it needs documentation and versions (ADR-038).

## Acceptance Criteria

- Each template records the data model version it was written for, and keeps rendering against that version after later releases.
- A refused template names the line and the cause.
- Templates can call only the documented functions, and none of them reads files, the network or the database.
- A rendered message larger than the sink platform accepts is refused at validation, not at delivery.
- Built-in templates ship with the binary and apply wherever a route has none of its own.
- Of the routes that reach one sink, the one with the narrowest source writes the sink's message with its templates: a repository before a group, a deeper group before a shallower one, a group before the instance, and the oldest route among equal ones (owner decision 2026-10-20).
- A thread keeps the route of its root while that route exists, so that its notices and its root refreshes keep one look. A change to that route's templates reaches the root at its next refresh.
- A stored template that fails at delivery gives way to the built-in one for that message, which is counted (owner decision 2026-10-20).

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests save valid and invalid templates through the API, compare previews with delivered messages, and render a template pinned to an older data model version after a version change.
