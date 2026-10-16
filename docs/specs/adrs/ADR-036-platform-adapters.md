---
id: "ADR-036"
type: architecture_decision_record
name: "Forges and chat platforms sit behind adapters"
description: "A neutral core handles merge requests, routes and threads. Everything platform-specific lives in one forge or chat adapter each."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-007"
  - "SYSARCH-008"
---

# ADR-036: Forges and chat platforms sit behind adapters

## Context

v0.1 has one parser for GitLab's payloads (`internal/event`) and one client for Mattermost (`internal/mattermost`). Its event model is still GitLab-shaped: a merge request is identified by GitLab's global id, reviewer progress comes from GitLab's approval rules, and pushes are attributed by GitLab project ids. Forgejo and Matrix come next, and Gitea, GitHub and Discord may follow.

## Decision

- A merge request is identified by its forge connection, its repository's stable id and its number. Paths are for display and scopes only, so renames do not break threads.
- Forge adapters verify, de-duplicate and translate deliveries into neutral events. Optionally they register hooks, read required approvals and link identities through OAuth.
- Chat adapters declare their capabilities: threads, edits, mentions, native format and command syntax. They create roots, reply, edit roots and receive commands.
- GitLab and Mattermost become the first adapters. The refactoring happens together with the Forgejo and Matrix adapters, because a second implementation shows where the seam really is.
- Where a forge lacks a value the board shows, the board leaves that part out (UC-007).

## Consequences

- Good: a new platform touches one adapter and its tests, not the core.
- Good: existing behaviour stays guarded by the e2e suite while the code moves (ADR-025).
- Bad: v0.1's threads cannot carry over, because they hold GitLab's global merge request id instead of the new identity. v0.2 therefore starts from an empty database (SYSREQ-043).
- On acceptance, ADR-014's plan is decided per sink, and SYSARCH-001 and SYSARCH-003 generalize into SYSARCH-007.
