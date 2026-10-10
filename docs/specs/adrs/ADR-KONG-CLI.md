---
id: "ADR-003"
type: architecture_decision_record
name: "Kong for the command line"
description: "The service command line and flag-to-environment configuration are built with alecthomas/kong."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# Architecture Decision: Kong for the command line

## Context and problem statement

The service is configured through command-line flags bound to environment variables
(SYSREQ-022). The decision is which CLI library — or none.

## Key factors

- Owner directive: any CLI is written with alecthomas/kong.
- Requirements: declarative struct tags, environment binding, help output, exit codes.

## Considered options

- alecthomas/kong.
- cobra (+viper).
- Standard library `flag`.

## Decision Outcome

Chosen option: "kong", by owner directive; it fits the requirement anyway — one struct declares
flags, env bindings, defaults, and validation with negligible runtime cost.

### Positive Consequences

- SYSREQ-022's option table is literally the kong struct; help text stays honest.
- Single dependency, no configuration-service layer (viper avoided).

### Negative Consequences

- Flag-vs-env precedence must be pinned in code and tested (flag wins; recorded in SYSREQ-022).

## Pros and Cons of the Options

### kong

- Good, because declarative tags keep config in one reviewable place.
- Bad, because precedence rules are opt-in and easy to get wrong silently.

### cobra + viper

- Bad, because viper's search-path magic is exactly the implicit behaviour 12-factor setups avoid.

### stdlib flag

- Good, because zero dependencies; bad, because env binding and validation become hand-rolled.

## Links

- Justifies SYSARCH-005; implements SYSREQ-022.
