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

- The OCI image and resource requests/limits are deployable, and a reachable PostgreSQL 18 with
  provisioned credentials is available (the schema is created by goose at startup).

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

Deployment posture after ADR-010: compute is stateless, so volumes and placement constraints are
gone. v1 still runs a single replica — worker claiming is not yet hardened for multi-process
concurrency — but the store no longer forbids more, and multi-replica claiming is a deliberate
future increment, not a v1 promise.

## Post-conditions

- Failed instances are restarted or replaced by the orchestrator without manual action.
- Queue backlog after a crash drains automatically on the next start.
