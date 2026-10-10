---
id: "SOL-001"
type: solution
name: "PatchCaster"
description: "A cloud native Go service that forwards GitLab 19+ merge request webhook events into Mattermost channels as one thread per merge request."
---

# Solution: PatchCaster

## Overview

PatchCaster is a cloud native HTTP service that ingests GitLab (19.0 and later) merge request
webhook deliveries — project, group, and system hooks alike — verifies their Standard Webhooks
HMAC signatures, filters them to an agreed action allowlist, routes each surviving event from its
GitLab project to a Mattermost channel, and posts the activity through the Mattermost bot account
REST API. Each merge request is one thread: the `open` action creates the root post and later
activity (`update`, approvals, `merge`, `close`) appears as replies inside it. Thread continuity
and duplicate suppression are kept in embedded SQLite, so the compute instance is restartable
with one small persistent volume.

## Business Context

Engineering teams already coordinate in Mattermost. The stock GitLab integration posts one flat
message per event into a firehose channel: no threading per merge request, no way to send several
projects into one team channel, and a static shared token for authentication. PatchCaster replaces
that with a per-merge-request conversation, a declarative routing table, and cryptographic
verification of every delivery using the signing tokens introduced in GitLab 19.

## Goals & KPIs

- **Goal**: Merge request activity is visible to a team without leaving Mattermost.
    - *KPI*: Median time from GitLab event creation to Mattermost post under 5 s at steady state.
- **Goal**: No forged or duplicated delivery reaches a channel.
    - *KPI*: 100 % of posted messages trace back to a delivery whose signature verified and whose `webhook-id` was unseen.
- **Goal**: No silent message loss.
    - *KPI*: The dropped-message counter stays at zero over a quarter; backlog drains to zero after a Mattermost outage.

## Stakeholders

- **Engineering teams**: subscribe to channel threads; judge noise vs. signal.
- **Platform operators**: deploy and monitor the workload; own the routing table rollout.
- **GitLab instance administrators**: register webhooks and signing tokens.
- **Mattermost administrators**: provision the bot account and channel membership.

## Constraints

- **Regulatory**: None specific applies *(assumed)*.
- **Physical & Design Context**: Implemented in Go with a `go` directive of at least 1.26, shipped as a single static binary in an OCI image (owner decision 2026-10-10; see SYSARCH-005).
- **Operational**: Requires GitLab 19.0+ with webhook signing tokens available (feature flag in 19.0, generally available in 19.1) and a Mattermost instance whose bot account API supports threaded posts. Runs as a 12-factor process: configuration from the command line and environment, TLS terminated at the ingress, one small persistent volume for state.
- **Safety & Security**: Signing secrets and the bot token are treated as secrets at rest and never appear in logs or metrics (SYSREQ-021). Every accepted delivery carries a verified signature (ADR-002).
- **Business**: Open source under the repository license; no revenue or time-to-market constraint recorded *(assumed)*.
