---
id: "UC-002"
type: use_case
name: "Configuration-driven channel routing"
description: "An operator declares which GitLab projects report into which Mattermost channels, including many projects sharing one channel."
refines:
  - "SOL-001"
status: retired
---

# Use Case: Configuration-driven channel routing

> **Retired on 2026-10-18.** UC-005 replaces it: teams route their sources into their sinks through the web interface and the API, and the routing file is gone (ADR-030).

## Actor(s)

- Platform operator (edits and ships the routing configuration).
- GitLab instance administrator (registers project, group, or instance-wide hooks).

## Pre-conditions

- The Mattermost channels exist and the bot account can post to them.
- The operator can deploy a new configuration (image or mounted file/ConfigMap) to the workload.

## Main Success Outcome

Every event from every configured GitLab project lands in the channel the operator assigned to
that project; several projects may share one channel; anything unconfigured is acknowledged
quietly and never posts.

## Key Functional Scope

- A declarative routing table keyed by GitLab project path, each entry naming a destination
  channel (SYSREQ-009); many entries may name the same channel (SYSREQ-011).
- Routing applies identically whether events arrive via per-project webhooks or one instance-wide
  system hook (SYSREQ-001, SCEN-007).
- Configuration is validated at startup; an invalid table stops the process before serving
  (SYSREQ-022).
- A routing change reaches only events accepted after it ships: queued messages keep the
  destination resolved at ingress (SYSREQ-024), so consistency needs no hot-reload.

Out of scope for v1:

- Hot-reloading the routing table without a process restart (a rollout is the update path).
- Any web UI or API for editing routes.
- Dynamic creation or resolution of channels by name (channel identifiers are operator-provided).

## Post-conditions

- Deployed configuration matches the reviewed revision (GitOps-visible).
- Metrics expose forwarded vs. ignored deliveries per decision, letting the operator confirm routing intent.
