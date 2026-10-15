---
id: "ADR-004"
type: architecture_decision_record
name: "log/slog with the tint handler"
description: "Use log/slog for all logging with lmittmann/tint for colored human-readable output."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-004"
---

# Architecture Decision: log/slog with the tint handler

## Context and problem statement

The service must produce structured, correlated logs without secrets (SYSREQ-020, SYSREQ-021).
The decision fixes the logging stack.

## Key factors

- Owner directive: `log/slog` with `lmittmann/tint` for colored logging.
- Correlation (SYSREQ-020) needs attribute-based records, not interpolated strings.
- Local development reads better in color than as JSON; clusters can grep either.

## Considered options

- `log/slog` + tint handler.
- `log/slog` + JSON handler.
- Third-party logger (zerolog/zap).

## Decision Outcome

Chosen option: "`log/slog` with the tint handler", by owner directive; tint keeps key-value
structure intact, which is what the correlation requirement actually consumes.

### Positive Consequences

- One logging API across the codebase (`slog.Logger` injected, never package-level globals).
- Swapping in the JSON handler later for a log pipeline is a `main`-level change only.

### Negative Consequences

- Colored output is pointless in cluster logs; mitigated by tint's colorless fallback when not a TTY.

## Pros and Cons of the Options

### slog + tint

- Good, because human-readable locally, structured everywhere.
- Bad, because machine parsing needs a handler swap — one line in `main`.

### Third-party logger

- Bad, because slog already covers the requirement and the directive removes the case.

## Links

- Justifies SYSARCH-004; implements SYSREQ-020/021.
