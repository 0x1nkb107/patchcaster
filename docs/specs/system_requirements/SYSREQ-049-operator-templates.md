---
id: "SYSREQ-049"
type: system_requirement
name: "Operator templates"
description: "Operators replace a route's built-in templates with their own, over a documented and versioned data model, after validation and a preview."
specification: "The service SHALL let operators replace the built-in root and notice templates of a route with their own Markdown templates over a documented and versioned data model, SHALL refuse a template that does not parse or does not render every sample event, and SHALL preview a template for each sink platform before it is saved."
status: proposed
derives_from:
  - "SCEN-021"
depends_on:
  - "SYSREQ-025"
---

# System Requirement: Operator templates

## Requirement Specification

> The service SHALL let operators replace the built-in root and notice templates of a route with their own Markdown templates over a documented and versioned data model, SHALL refuse a template that does not parse or does not render every sample event, and SHALL preview a template for each sink platform before it is saved.

## Rationale

The owner wants teams to decide how messages look (2026-10-15). Once operators write templates, the data model they write against becomes a public contract, so it needs documentation and versions (ADR-038).

## Acceptance Criteria

- Each template records the data model version it was written for, and keeps rendering against that version after later releases.
- A refused template names the line and the cause.
- Templates can call only the documented functions, and none of them reads files, the network or the database.
- A rendered message larger than the sink platform accepts is refused at validation, not at delivery.
- Built-in templates ship with the binary and apply wherever a route has none of its own.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests save valid and invalid templates through the API, compare previews with delivered messages, and render a template pinned to an older data model version after a version change.
