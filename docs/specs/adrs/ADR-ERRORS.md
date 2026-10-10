---
id: "ADR-005"
type: architecture_decision_record
name: "errors.Is, errors.As, and errors.AsType for error handling"
description: "Classify and unwrap errors with the standard library sentinel, As, and generic AsType APIs."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# Architecture Decision: errors.Is, errors.As, and errors.AsType for error handling

## Context and problem statement

Component boundaries (verifier → handler → store → Mattermost client) must classify failures
deterministically: retryable vs. terminal, auth failure vs. transport failure (SYSREQ-016 picks
between them). The decision fixes the error-handling idiom.

## Key factors

- Owner directive: use `errors.Is`, `errors.As`, and `errors.AsType`.
- Go 1.26 ships the generic `errors.AsType[T]`, removing the pointer-to-variable ceremony.
- Policy: wrap with `%w`, sentinel values and typed errors at package boundaries, no error-string
  matching, message style per repository conventions.

## Considered options

- Standard library sentinel/As/AsType idiom.
- String matching on error text.
- Panic/recovery-based failure propagation.

## Decision Outcome

Chosen option: "sentinel errors with `errors.Is`, and `errors.As`/`errors.AsType` for typed
errors", because classification then survives wrapping, is checked by the compiler at the type
level, and keeps the retry/terminal decision in SYSARCH-003 data-driven.

### Positive Consequences

- Typed errors (for example a `RateLimitedError` carrying the reset instant) cross the store
  boundary intact for `AsType` extraction.
- SYSREQ-021 stays easy: typed error `Error()` methods control their own secret-free text.

### Negative Consequences

- Discipline required: an unexported error type cannot be matched at a boundary — exported
  errors get doc comments stating they are API.

## Pros and Cons of the Options

### stdlib idiom

- Good, because zero dependencies and universal reader familiarity.
- Good, because `errors.AsType` makes extraction expressions, not out-params.

### string matching

- Bad, because it breaks on the next message reword — which the error-style policy invites.

## Links

- Justifies SYSARCH-005 (cross-cutting convention).
