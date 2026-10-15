---
id: "ADR-009"
type: architecture_decision_record
name: "text/template for message rendering"
description: "Message bodies are rendered with the Go standard library text/template package over an escaped view model."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-003"
---

# Architecture Decision: text/template for message rendering

## Context and problem statement

SYSREQ-025 requires templated message bodies and the owner directive names the Go standard
library choice: `html/template` or `text/template`, whichever is more appropriate. Message text
is Markdown destined for a JSON API field — picking the wrong escape grammar either corrupts the
text or leaves injection implicit.

## Key factors

- Mattermost post bodies are Markdown strings, not HTML (ADR-002 content policy, SYSREQ-003).
- Wire-level escaping is JSON encoding, owned by the API client boundary, not the template.
- Untrusted payload fields (titles, branch names) must be neutralized in Markdown grammar, which
  neither library does automatically.

## Considered options

- `text/template` with an escaped view model.
- `html/template`.
- Direct `fmt` composition without templates.

## Decision Outcome

Chosen option: "`text/template`", because it is the appropriate grammar: templates emit
Markdown, and `html/template`'s contextual escaper targets HTML output — it would mangle
`**bold**`, URLs, and link syntax while still not protecting the Markdown layer, which needs a
purpose-built escaper regardless.

### Positive Consequences

- Standard library, zero dependencies; templates parsed once at startup (SYSREQ-025).
- The view model exposes only escaped accessors, so Markdown neutralization lives in one FuncMap
  shared by all templates — where SYSREQ-003's adversarial tests already look.

### Negative Consequences

- No automatic context escaping: a future template author could reach for a raw field — mitigated
  by the view model design (raw values are not present) plus golden adversarial fixtures.

## Pros and Cons of the Options

### text/template + escaped view model

- Good, because output grammar matches the destination.
- Good, because escaping policy stays in one reviewable place.
- Bad, because protection is opt-in per accessor rather than library-enforced.

### html/template

- Bad, because it escapes for HTML into a Markdown/JSON destination — wrong grammar on both ends.

### fmt composition

- Bad, because message shape becomes string concatenation scattered through code, defeating
  SYSREQ-025's tunability goal.

## Links

- Justifies SYSARCH-003; implements SYSREQ-025 and SYSREQ-003.
- [text/template](https://pkg.go.dev/text/template)
