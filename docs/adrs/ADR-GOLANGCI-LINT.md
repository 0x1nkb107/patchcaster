---
id: "ADR-011"
type: architecture_decision_record
name: "golangci-lint as the static-analysis gate"
description: "Code quality is enforced by a version-pinned golangci-lint run, exposed as just lint and blocking merges in CI."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# Architecture Decision: golangci-lint as the static-analysis gate

## Context and problem statement

The repository policies that matter most here — error handling (ADR-005), determinism
(ADR-006), secret hygiene (SYSREQ-021) — are conventions until something enforces them. The
owner directive (2026-10-10) fixes the enforcement tool: `golangci-lint`, integrated as a code
quality gate.

## Key factors

- One runner for many analyzers beats a CI pile of bespoke steps: one version, one config file,
  one `just lint` invocation shared by humans and CI (ADR-007).
- Version pinning per the repository's reproducibility policy: CI and local runs must agree.
- Config uses the golangci-lint v2 schema; greenfield tree, so it runs over everything — no
  legacy baseline needed.
- Linter set chosen to mirror the graph's decrees rather than to maximize noise.

## Considered options

- `golangci-lint` with a curated linter set.
- Bare `go vet` + `staticcheck` wired separately into CI.
- Bespoke per-analyzer CI steps without a local equivalent.

## Decision Outcome

Chosen option: "golangci-lint, version-pinned, config `.golangci.yml` (v2 schema), gate =
`just lint` locally and a required status check in CI before merge", by owner directive.

Curated set beyond the default analyzers (`errcheck`, `govet`, `ineffassign`, `staticcheck`,
`unused`):

- `errorlint` — mechanically enforces ADR-005: rejects `err.Error()` comparisons, requires
  `errors.Is` / `errors.As` / `errors.AsType` and `%w` wrapping.
- `nolintlint` — a suppression must be machine-readable and explained, or CI fails; keeps the
  escape hatches honest.
- `gofmt` under the v2 `formatters` section — stdlib shape, no formatting religion.

### Positive Consequences

- ADR-005 stops being a style preference: string-matching errors cannot merge.
- Local and CI quality verdicts are identical (same pinned version, same config, same entry
  point `just lint`).
- Suppression debt stays visible and greppable via `nolintlint` rules.

### Negative Consequences

- A toolchain dependency whose own Go build version can drift from the project's — mitigated by
  pinning the same version in the CI action and the justfile recipe (single source of truth in
  the CI config).
- Analyzer upgrades can surface new findings retroactively — mitigated by the version pin;
  upgrades are deliberate, reviewable changes.

## Pros and Cons of the Options

### golangci-lint (chosen)

- Good, because one pinned tool version covers vet, staticcheck, errorlint, and formatting.
- Good, because the config file is reviewable like any code.
- Bad, because curated-set drift over releases needs managed upgrades.

### Bare vet + staticcheck

- Good, because fewer knobs; bad, because policy linters (errorlint, nolintlint) get bolted on
  ad hoc anyway.

### Bespoke CI steps

- Bad, because local runs and CI drift apart — the exact gap suppression folklore grows in.

## Links

- Justifies SYSARCH-005; enforces ADR-005 mechanically; complements ADR-006 and ADR-007.
- Note: the arrange-act-assert rule of ADR-006 is intentionally *not* linted — it is covered by
  test review, not static analysis.
- [golangci-lint](https://golangci-lint.run/)
