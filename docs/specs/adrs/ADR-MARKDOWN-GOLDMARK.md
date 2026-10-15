---
id: "ADR-037"
type: architecture_decision_record
name: "Markdown is the common format, parsed by goldmark and rendered per platform"
description: "Templates produce CommonMark with GFM extensions. goldmark parses it once, and one renderer per chat platform produces that platform's format."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-008"
---

# ADR-037: Markdown is the common format, parsed by goldmark and rendered per platform

## Context

The owner made Markdown the lingua franca: templates are written in Markdown, and PatchCaster renders HTML where a sink needs it (2026-10-15). Mattermost takes Markdown, and Matrix takes HTML with a plain-text fallback. Mentions differ on every platform.

## Decision

- The dialect is CommonMark with the GitHub Flavored Markdown extensions for tables, strikethrough and autolinks.
- `github.com/yuin/goldmark/v2` parses each rendered template into a syntax tree (owner decision 2026-10-15). Version v2.1.6 is MIT licensed, complies with CommonMark 0.31.2, and ships the tables, strikethrough and linkify extensions of the GFM dialect.
- A mention is a node type of its own, produced only by the templates' `mention` function (SYSREQ-054).
- One renderer per platform walks the tree. Mattermost receives Markdown again. Matrix receives HTML limited to the tags its specification permits, plus a plain-text body.
- Raw HTML in a template is rendered as text. Link targets are limited to `http`, `https` and `mailto`.

## Consequences

- Good: templates stay platform-neutral, and every platform's output is safe by construction instead of by string cleaning.
- Good: adding a platform means one renderer.
- Bad: a platform's own Markdown extras, beyond the common dialect, are not available to templates.
- Bad: goldmark's README calls v2 "still in the early stages of release", and some third-party extensions support only v1. PatchCaster needs only the built-in GFM extensions and its own mention node, so it depends on no third-party extension.
