---
id: "SCEN-007"
type: scenario
name: "An instance-wide system hook drives routing"
description: "One system hook feeds every repository on the instance and the routing table decides what is forwarded."
refines:
  - "UC-002"
---

# Scenario: An instance-wide system hook drives routing

## Overview

A GitLab instance administrator registers one system hook instead of per-project webhooks; the
routing table alone decides which repositories reach channels.

## Initial State

- The system hook URL and signing token are configured instance-wide.
- Only some of the instance's projects appear in the routing table.

## Trigger

Merge requests are opened in one configured and one unconfigured project; each is delivered from
the system hook.

## Step-by-Step Flow

1. The service parses system-hook deliveries (which carry an `event_name` alongside the merge request object).
2. Routing looks up `project.full_path` exactly as for project webhooks.
3. The configured project's event is forwarded; the unconfigured one is acknowledged quietly.

## Expected Outcome

- Identical forwarding behaviour and thread continuity as with project webhooks.
- No per-project GitLab administration beyond the single hook.

## Exceptions & Edge Cases

- The shared signing token means one secret gates the whole instance's traffic; rotation is an
  instance-admin operation.
