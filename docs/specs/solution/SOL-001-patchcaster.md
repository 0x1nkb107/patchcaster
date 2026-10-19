---
id: "SOL-001"
type: solution
name: "PatchCaster"
description: "A cloud native Go service that forwards GitLab 19+ merge request webhook events into Mattermost channels as one thread per merge request."
---

# Solution: PatchCaster

## Overview

PatchCaster is a cloud native HTTP service that ingests GitLab (19.0 and later) merge request webhook deliveries — project, group, and system hooks alike — verifies their Standard Webhooks HMAC signatures, filters them to an agreed action allowlist, routes each surviving event to the Mattermost channels that its teams' routes name, and posts the activity through the Mattermost bot account REST API. Each merge request is one thread per channel: the `open` action creates the root post and later activity (`update`, approvals, `merge`, `close`) appears as replies inside it. Since v0.2, the connections, teams, sources, sinks and routes live in the database and are managed through a web interface and an API (UC-004, UC-005). Thread continuity, duplicate suppression and the configuration are kept in an external PostgreSQL 18 (ADR-010), so compute instances are stateless and restartable at will.

## Business Context

Engineering teams already coordinate in Mattermost. The stock GitLab integration posts one flat
message per event into a firehose channel: no threading per merge request, no way to send several
projects into one team channel, and a static shared token for authentication. PatchCaster replaces
that with a per-merge-request conversation, routes that each team manages itself, and cryptographic
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
- **Instance Admins** (IT department): connect the platforms and the login, set up teams, and decide who may do what (UC-004).
- **Routing Managers** (team leads): route their teams' sources into their sinks (UC-005).
- **Regular Users** (individual developers): manage their own identity links and tokens (UC-006).
- **Infrastructure team**: deploys and monitors the workload (UC-003).
- **GitLab administrators**: grant PatchCaster an access token, or register webhooks and signing tokens by hand.
- **Mattermost administrators**: provision the bot account and channel membership.

## Constraints

- **Regulatory**: None specific applies *(assumed)*.
- **Physical & Design Context**: Implemented in Go with a `go` directive of at least 1.26, shipped as a single static binary in an OCI image (owner decision 2026-10-10; see SYSARCH-005).
- **Operational**: Requires GitLab 19.0+ with webhook signing tokens available (feature flag in 19.0, generally available in 19.1) and a Mattermost instance whose bot account API supports threaded posts. Runs as a 12-factor process: startup and operating settings from the command line and environment, TLS terminated at the ingress; state and configuration live in an external PostgreSQL 18 (ADR-010, ADR-030).
- **Safety & Security**: Credentials and signing tokens are sealed with the instance key at rest and never appear in logs, metrics or API answers (SYSREQ-021, SYSREQ-042). Every accepted delivery carries a verified signature (ADR-002).
- **Business**: Open source under the repository license; no revenue or time-to-market constraint recorded *(assumed)*.

## Evolution

On 2026-10-15 the owner decided to grow PatchCaster beyond one forge and one chat platform. The use cases UC-004 to UC-007 describe the direction. One instance serves one organization and connects one or more forges, starting with Forgejo, to one or more chat platforms, starting with Matrix including encrypted rooms. Sources, sinks and the routes between them live in the database and are managed through a web interface and an API with a published OpenAPI description. Instance Admins, Routing Managers and Regular Users share the work. The first increment, v0.2, brought the configuration database, the management API and the web interface's foundation for GitLab and Mattermost. The later increments stay proposed until they are built.

### Increments

The owner decided on 2026-10-16 to build the evolution in five increments, each shipped as its own release, starting with v0.2. Each increment accepts its proposed items in the pull request that implements them. When an increment spans several pull requests, its items are accepted together in the last one, so that no accepted item rests on a proposed one. Scenarios that use Forgejo or Matrix as their example hold for GitLab and Mattermost from the first increment, and in full once those adapters exist.

1. **Foundation (v0.2):** the neutral core and the configuration database behind the management API, for GitLab and Mattermost only, with built-in templates. Login, roles, teams, API tokens, the audit log, encrypted secrets and health metrics come with it, and so does the web interface's foundation with its setup, login, token and health pages. It accepts UC-004, UC-005's routing part, SCEN-013 to SCEN-020, SCEN-022, SCEN-029, SYSREQ-034 to SYSREQ-048, SYSREQ-062, SYSREQ-064, SYSARCH-006 to SYSARCH-008, ADR-029 to ADR-036 and ADR-042 to ADR-045, each for GitLab and Mattermost and without the parts that later increments bring. It was accepted on 2026-10-18.
2. **Web interface and templates:** the full management interface, and custom templates with their preview, rendered through Markdown. It accepts SCEN-021, SYSREQ-049 to SYSREQ-051, ADR-037 and ADR-038, and the template parts of UC-005, SYSARCH-006 and SYSARCH-008.
3. **Forgejo:** the Forgejo adapter. It accepts SCEN-027, SYSREQ-056 to SYSREQ-058 and ADR-041, and the Forgejo parts of SYSREQ-044 and SYSARCH-007.
4. **Encrypted Matrix:** the Matrix adapter with encrypted rooms and its HTML renderer. It accepts UC-007, SCEN-028, SYSREQ-059 to SYSREQ-061 and ADR-040, and the Matrix parts of SCEN-014, SYSREQ-045, SYSREQ-064, SYSARCH-007 and ADR-035.
5. **Identity linking and mentions:** chat and forge identity links, the chat commands, and mentions in threads. It accepts UC-006, SCEN-023 to SCEN-026, SYSREQ-052 to SYSREQ-055, SYSREQ-063 and ADR-039, and the parts on links, chat commands and mentions of SCEN-014, SYSREQ-036, SYSREQ-064 and SYSARCH-006 to SYSARCH-008.
