---
id: "UC-003"
type: use_case
name: "Cloud native operation"
description: "Platform operators run the bot as a containerized workload with health, metrics, logging, and shutdown behaviour that Kubernetes manages safely."
refines:
  - "SOL-001"
---

# Use Case: Cloud native operation

## Actor(s)

- Kubernetes (or equivalent) orchestrator: probes, restarts, rollouts.
- Platform operators: watch metrics and logs.

## Pre-conditions

- The OCI image, resource requests/limits, and a persistent volume for state are deployable in the target cluster.

## Main Success Outcome

The service starts deterministically from configuration, answers probes truthfully, exposes
delivery health as metrics, logs structured correlated records, and drains in-flight deliveries
within a bounded window during rollouts.

## Key Functional Scope

- Liveness and readiness endpoints that reflect real capability (SYSREQ-018).
- Prometheus metrics for the ingest-to-post funnel (SYSREQ-019).
- Structured, correlated logs free of secrets (SYSREQ-020, SYSREQ-021).
- Startup configuration validation with a non-zero exit on invalid input (SYSREQ-022).
- SIGTERM drain bounded to 30 s (SYSREQ-023).

Consequences of ADR-001 to accept at deployment: the SQLite volume is ReadWriteOnce, so the
workload runs as a single replica; horizontal scaling would require a different store first.

## Post-conditions

- Failed instances are restarted or replaced by the orchestrator without manual action.
- Queue backlog after a crash drains automatically on the next start.
