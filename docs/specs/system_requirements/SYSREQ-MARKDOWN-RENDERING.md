---
id: "SYSREQ-050"
type: system_requirement
name: "Markdown as the common message format"
description: "Templates produce Markdown, and PatchCaster renders it into each sink platform's native format."
specification: "The service SHALL treat template output as CommonMark with the GitHub Flavored Markdown extensions for tables, strikethrough and autolinks, and SHALL render it into each sink platform's native format, sending Matrix both HTML restricted to the tags the Matrix specification permits and a plain-text body."
status: proposed
derives_from:
  - "SCEN-021"
  - "SCEN-028"
---

# System Requirement: Markdown as the common message format

## Requirement Specification

> The service SHALL treat template output as CommonMark with the GitHub Flavored Markdown extensions for tables, strikethrough and autolinks, and SHALL render it into each sink platform's native format, sending Matrix both HTML restricted to the tags the Matrix specification permits and a plain-text body.

## Rationale

The owner made Markdown the lingua franca (2026-10-15). One parsed document rendered once per platform keeps templates platform-neutral, and it avoids rewriting strings for each dialect (ADR-037).

## Acceptance Criteria

- Mattermost receives Markdown.
- Matrix receives `org.matrix.custom.html` with only permitted tags, plus a plain-text `body`.
- Raw HTML in a template is rendered as text, never passed through.
- Constructs a platform cannot show degrade in a documented way, the same way every time.

## Verification Plan

- **Method**: Test.
- **Procedure**: Golden tests render a fixed set of documents for every platform. A property test asserts that Matrix HTML never contains a tag outside the permitted set.
