---
id: "SYSREQ-045"
type: system_requirement
name: "Sinks"
description: "A sink names one chat connection and one channel or room, and PatchCaster checks it can post there."
specification: "The service SHALL deliver to sinks that each name one chat connection and one channel or room, and SHALL verify when a sink is created that the bot can post into it."
status: proposed
derives_from:
  - "SCEN-015"
  - "SCEN-028"
---

# System Requirement: Sinks

## Requirement Specification

> The service SHALL deliver to sinks that each name one chat connection and one channel or room, and SHALL verify when a sink is created that the bot can post into it.

## Rationale

A sink that cannot be posted to would only show up as failed deliveries later. Checking at creation shows the problem to the administrator who can fix it.

## Acceptance Criteria

- For Mattermost, the bot must be a member of the channel.
- For Matrix, the bot joins a room it was invited to and must be allowed to send messages there.
- Each sink shows its state: deliverable, or the reason it is not.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests against the fake platforms cover a deliverable sink and each refusal reason.
