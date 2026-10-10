---
id: "SCEN-008"
type: scenario
name: "Unhealthy instances leave rotation"
description: "Readiness fails while the service cannot do its job, so traffic is steered to healthy replicas."
refines:
  - "UC-003"
---

# Scenario: Unhealthy instances leave rotation

## Overview

The orchestrator learns the truth about each instance: an instance that cannot serve does not
receive traffic or claims of health.

## Initial State

- The workload is deployed with liveness and readiness probes wired to the internal listener.

## Trigger

Any of: startup with an invalid routing table or missing bot token; loss of write access to the
SQLite volume after startup.

## Step-by-Step Flow

1. At startup, configuration validation fails, so the process exits non-zero before listening.
2. Mid-life volume failure makes readiness report failure while the process stays alive long enough for the orchestrator to observe it.
3. The orchestrator stops routing to the instance and restarts or replaces it.

## Expected Outcome

- Readiness is 200 only when configuration is valid and the state store is writable.
- Recovery after the volume returns brings readiness back to 200 without manual restart.

## Exceptions & Edge Cases

- Liveness must not depend on Mattermost availability: a Mattermost outage is backlog, not
  unhealth; restarting the pod would only lose in-flight state.
