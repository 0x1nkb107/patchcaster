---
id: "ADR-012"
type: architecture_decision_record
name: "zog for configuration validation"
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---
# Architecture Decision: zog for configuration validation

## Context and problem statement

SYSREQ-022 requires that invalid configuration makes the service exit non-zero
**before listening**, naming the offending options. kong (ADR-003) already owns
presence, parsing, types, and defaults; what remains is *semantic* validation of
the parsed struct (positive durations, enumerated log level, usable signing
secrets, and the replay-loop invariant that seen-retention exceeds the event
freshness cap). Until now these were hand-written checks that abort at the first
violation. The owner directive of 2026-10-10 adopts `github.com/Oudwins/zog` for
this validation layer.

## Key factors

- Owner directive (2026-10-10): use Zog (Oudwins/zog) for validation.
- Values reach validation already typed and defaulted by kong; the job is
  asserting an in-hand struct, not coercing raw input — Zog's `Validate` path
  fits exactly, and its `Parse`/coercion path must not be used (parsing-over-
  policing: kong is the single parse boundary).
- Operator experience: reporting *every* invalid option in one exit message is
  materially better for first deployment than the current first-error abort.
- Dependency policy: Zog has zero transitive dependencies.
- Boundary: webhook payload handling (`event.Parse`) stays hand-written — wire
  payloads are authenticated by signature (ADR-002) and normalized by parsing,
  not schema-policed; applying a validator there would be a second parser.

## Considered options

- Keep hand-written `Validate` checks.
- Oudwins/zog schemas.
- go-playground/validator (struct tags + reflection).
- Omit semantic validation entirely.

## Decision Outcome

Chosen option: "Oudwins/zog", by owner directive. The parsed `ServeCmd` struct
is validated against one `z.Struct` schema composed at init time: `OneOf` for
the log level, `Required` non-empty slices of non-empty strings for signing
secrets, and `z.CustomFunc` tests for positive durations (v0.23 has no native
duration schema). The cross-field replay-loop invariant is expressed as a
struct-wide custom schema so it lands in the same aggregated issue list.
Issues are rendered through a flag-name map (Go field name → CLI flag) and
joined with `errors.Join`, so kong's exit message names every offending option
at once. The dependency is pinned (v0.23.0 at adoption) and exercised by the
lint/test gates on every bump. A second owner directive of the same day extends
the tool to the decoded routing table (`internal/routing`): per-row `Required`
fields plus a whole-document custom schema for project uniqueness, so every
broken row is named with its index in one startup error.

### Positive Consequences

- All configuration violations are reported together, each naming its flag —
  strict improvement to SYSREQ-022's operator experience.
- Validation intent reads as a declarative table next to the kong tags it
  constrains, replacing a stack of if-statements.
- Zero transitive dependencies; the validation vocabulary (schemas, issues)
  stays at the startup-configuration boundaries — CLI flags and the routing
  file — and reaches no runtime path.

### Negative Consequences

- Zog is pre-1.0: minor versions may break the API. Mitigated by the pinned
  version and the CI gate, which compiles and runs every schema test on any
  bump.
- v0.23 offers no struct-level tests and no duration schema, so the
  cross-field rule, duration positivity, and the routing table's cross-row
  uniqueness rule go through `CustomFunc` — slightly
  more ceremony than a tagged validator would demand.
- Two validation idioms coexist in the codebase by design: schema-based at the
  startup-configuration boundaries (CLI, routing file), hand-written parsing at
  the webhook edge (see Key factors for why).

## Pros and Cons of the Options

### Hand-written checks (status quo ante)

- Good, because zero dependencies and no new idiom.
- Bad, because first-error aborts make first deploys iterative; the checks
  duplicate what a schema states directly.

### Oudwins/zog

- Good, because Zod-style chaining reads as a table of invariants with rich,
  path-addressed issues.
- Good, because aggregation is native to the error model.
- Bad, because the API is pre-v1 and struct-level tests are missing today.

### go-playground/validator

- Good, because it is battle-tested and tag-declarative.
- Bad, because it validates via reflection over tags, splitting the invariant
  table from the typed code, and pulls a dependency tree Zog does not have.

### Omit semantic validation

- Bad, because invalid configuration would reach runtime (SYSREQ-022 violation).

## Links

- Justifies SYSARCH-005; realizes SYSREQ-022 acceptance (names offending
  options, exits before listening).
- Complements ADR-003 (kong parses; zog asserts) and ADR-007 (joined errors
  keep `errors.Is`/`errors.As` semantics).
- [Oudwins/zog](https://github.com/Oudwins/zog)
