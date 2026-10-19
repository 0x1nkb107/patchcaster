---
id: "SYSARCH-008"
type: system_architecture
name: "Routing and rendering core"
description: "Accepted events fan out along routes into per-sink queues, and messages render from operator templates into Markdown, then into each platform's format."
satisfies:
  - "SYSREQ-046"
  - "SYSREQ-047"
  - "SYSREQ-048"
  - "SYSREQ-049"
  - "SYSREQ-050"
  - "SYSREQ-051"
  - "SYSREQ-054"
---

# System Architecture: Routing and rendering core

Accepted in part on 2026-10-18, with increment 1 (SOL-001). Still planned, and not normative until their increment is accepted: rendering through Markdown from a versioned view model (ADR-037, ADR-038), with increment 2, its HTML form for Matrix, with increment 4, and mentions, with increment 5. Until then, the templates of SYSARCH-003 render the messages.

Normative: routing decides per sink at acceptance, and rendering turns a versioned view model into Markdown and then into each platform's native format.

## Technical Strategy

At acceptance, ingest resolves the routes of the matched sources. For each distinct sink it decides the delivery plan: whether to reply, which is the only part a filter changes, and to refresh the board (ADR-014). It enqueues one message per sink. The queue's ordering holdback and the thread mapping are keyed by merge request and sink (SYSREQ-048).

At delivery, the worker builds the view model, with every forge value already escaped (SYSREQ-051). It applies the route's template with `text/template` (ADR-009, ADR-038) and parses the Markdown result with goldmark v2. The sink platform's renderer then produces the platform format (ADR-037). Mentions are nodes in the document, resolved against identity links when the message is delivered (SYSREQ-054).

## Static View (Structure)

- **Router**: matched sources to routes to sinks, de-duplicated per sink.
- **Planner**: the delivery plan per sink, with filters applied to replies only.
- **ViewModel**: one versioned data model per template version.
- **Escaper**: forge values to Markdown-safe text.
- **TemplateEngine**: `text/template` with a restricted function map, including `mention`.
- **MarkdownDocument**: the goldmark v2 syntax tree, extended with mention nodes.
- **Renderers**: one per platform. Mattermost emits Markdown, Matrix emits restricted HTML plus plain text.

## Dynamic View (Behavior)

One accepted event becomes up to one queued message per sink. Each message renders when it is delivered, so a template change applies to every message delivered after it. Retries render again and produce the same output for the same state.

## Notes

Accepted in part on 2026-10-18. It changed SYSARCH-002's keys. SYSARCH-003's rendering changes with increment 2.
