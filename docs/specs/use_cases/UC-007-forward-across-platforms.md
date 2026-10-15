---
id: "UC-007"
type: use_case
name: "Activity across forges and chat platforms"
description: "Teams follow merge request activity from GitLab and Forgejo in Mattermost channels and Matrix rooms, encrypted rooms included, with the same thread and board behaviour everywhere."
status: proposed
refines:
  - "SOL-001"
---

# Use Case: Activity across forges and chat platforms

## Actor(s)

- Engineering teams, who read the threads.
- Administrators, who connect the platforms (UC-004).

## Pre-conditions

- The forge and chat connections exist, and an operator routed the sources into the sinks (UC-005).

## Main Success Outcome

A Forgejo pull request behaves like a GitLab merge request: one thread per sink, a live root board, and replies for notable activity. In a Matrix room the replies form a Matrix thread and the board updates through edits. Encrypted rooms behave like unencrypted ones.

## Key Functional Scope

- Forgejo deliveries verified, de-duplicated and capped by age (SYSREQ-056, SYSREQ-057).
- Forgejo activity mapped onto the merge request events that drive threads and boards (SYSREQ-058).
- Matrix threads and edits (SYSREQ-059), in encrypted rooms too (SYSREQ-060, SYSREQ-061).

Out of scope:

- Gitea, GitHub and Discord. They come later on the same adapter model (ADR-036).
- Matrix rooms the bot was not invited to.

## Post-conditions

- Where a forge lacks a value the board shows, such as required approvals without API access, the board leaves that part out instead of guessing.
