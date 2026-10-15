---
id: "SYSREQ-044"
type: system_requirement
name: "Sources and webhook registration"
description: "A source selects repositories of a forge connection by scope. Each connection has one webhook endpoint, and PatchCaster registers the hooks it needs."
specification: "The service SHALL accept webhooks for each forge connection at an endpoint of its own, verified with that connection's secrets; SHALL pass each verified event to every source of that connection whose scope contains the event's repository; and SHALL register, through the forge API where the connection has API access, one hook for each source scope not already covered by a broader registered scope."
status: proposed
derives_from:
  - "SCEN-015"
  - "SCEN-027"
---

# System Requirement: Sources and webhook registration

## Requirement Specification

> The service SHALL accept webhooks for each forge connection at an endpoint of its own, verified with that connection's secrets; SHALL pass each verified event to every source of that connection whose scope contains the event's repository; and SHALL register, through the forge API where the connection has API access, one hook for each source scope not already covered by a broader registered scope.

## Rationale

The endpoint and secret belong to the connection, and sources only select repositories. One forge hook therefore serves every source inside its scope. An instance-wide GitLab system hook keeps working as in v0.1 (SCEN-007). Overlapping hooks would deliver the same event twice. Registering only scopes that no broader hook covers prevents that.

## Acceptance Criteria

- Scopes are an instance, an organization or group, or a single repository.
- An event whose repository lies in no source's scope is acknowledged without effect, as in SYSREQ-010.
- Registering a broader scope removes the narrower hooks it now covers. Removing it re-registers the narrower ones still needed.
- Without API access, the connection shows its endpoint and secret once, when it is created, for manual registration.
- A connection may hold several secrets at once, so a secret can rotate without downtime.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests against a fake forge API assert the registrations for nested and overlapping scopes, and that one delivery reaches every matching source exactly once.
