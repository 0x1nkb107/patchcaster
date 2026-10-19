---
id: "ADR-013"
type: architecture_decision_record
name: "YAML-org go-yaml (go.yaml.in) for YAML"
status: superseded
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# Architecture Decision: YAML-org go-yaml (go.yaml.in) for YAML

> **Superseded by ADR-030 (2026-10-18).** The routing file, the only YAML that PatchCaster read, is gone with the configuration's move into the database.

## Context and problem statement

The routing table (SYSREQ-009) is the service's only YAML input. It was
initially parsed with `gopkg.in/yaml.v3`. The upstream `go-yaml/yaml` repository
was labeled unmaintained in April 2025, and the `gopkg.in/yaml.v3` line is now
frozen to security fixes only. The YAML organization maintains the successor
project at `github.com/yaml/go-yaml`, published under the import path
`go.yaml.in/yaml/v4` — the owner directive of 2026-10-10 adopts it.

## Key factors

- Owner directive (2026-10-10): process YAML with the YAML-org go-yaml.
- Maintenance posture: the org's maintainer team includes representatives of
  the largest downstream forks; `gopkg.in/yaml.v3` receives security fixes only.
- API compatibility: `go.yaml.in/yaml/v4` keeps the v3 API (`Unmarshal` plus
  `yaml:` struct tags), so the swap is one import in the routing loader — no
  new abstraction and no vendored parser code.
- Dependency policy: pure Go, no transitive deps, MIT/Apache-2.0 — same
  posture as the frozen v3.
- Scope honesty: there is exactly one YAML document in this system. Whatever is
  chosen stays confined to the routing loader.

## Considered options

- go.yaml.in/yaml/v4 (YAML-org successor).
- gopkg.in/yaml.v3 (status quo ante; frozen upstream).
- goccy/go-yaml (different lineage, richer feature surface).
- Hand-rolled parsing of the routing file.

## Decision Outcome

Chosen option: "go.yaml.in/yaml/v4", by owner directive. Pinned at
`v4.0.0-rc.6` — the highest published v4 — because v4 is the only actively
developed line; the final `v4.0.0` release supersedes the pin, and the CI
dependency path makes the bump visible. Confined to `internal/routing`:
`routing.Parse` keeps decoding and validation as separate steps — go-yaml
parses, a Zog schema asserts the table's invariants on the decoded struct
(ADR-012), and the loader builds the lookup map (parsing over policing).

### Positive Consequences

- The routing table parser has an active maintainer with organizational
  backing rather than a frozen upstream.
- Drop-in API: the loader's logic and tests are unchanged; the diff is the
  import and error text quality, which v4 improves (positioned errors).

### Negative Consequences

- v4 is release-candidate-stage: the pin is a prerelease. Mitigated by the tiny
  surface used (one `Unmarshal` call) and by the CI gate running the routing
  tests on every dependency change.

## Pros and Cons of the Options

### go.yaml.in/yaml/v4

- Good, because it is the directed project and the only line with ongoing
  feature development.
- Bad, because the stable v4.0.0 tag does not exist yet.

### gopkg.in/yaml.v3

- Good, because it is battle-tested and already vendored.
- Bad, because its upstream is explicitly unmaintained; security-only fixes
  leave functional bugs to us.

### goccy/go-yaml

- Good, because it is feature-rich (node APIs, path queries).
- Bad, because those features are unused here and the org successor is the
  directed choice.

## Links

- Justifies SYSARCH-005; parses the table required by SYSREQ-009.
- [yaml/go-yaml](https://github.com/yaml/go-yaml) (`go.yaml.in/yaml/v4`)
