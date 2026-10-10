---
id: "ADR-007"
type: architecture_decision_record
name: "justfile for common tasks"
description: "Common project tasks are exposed through a justfile."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# Architecture Decision: justfile for common tasks

## Context and problem statement

Build, test, lint, migration, and container tasks need one discoverable entry point for humans
and CI alike.

## Key factors

- Owner directive: add a justfile for common project tasks.
- Tasks must not hide meaning: every recipe visible in the file, no recursive make sorcery.

## Considered options

- `just` with a justfile.
- GNU make.
- Ad-hoc shell scripts in `scripts/`.

## Decision Outcome

Chosen option: "justfile", by owner directive; `just` is a task runner, not a build system, so
recipes stay one-line honest and tab-completable.

### Positive Consequences

- `just`, `just test`, `just lint`, `just migrate`, `just container` as the shared vocabulary;
  CI invokes the same recipes.

### Negative Consequences

- Contributors need `just` installed — a one-line dependency documented in the README.

## Pros and Cons of the Options

### justfile

- Good, because recipes are plain shells with parameters, no implicit rules.
- Bad, because one more tool to install (make would need no install but invites build-system drift).

## Links

- Justifies SYSARCH-005.
