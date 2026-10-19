---
id: "SYSREQ-036"
type: system_requirement
name: "Management listener"
description: "The web interface and the management API are served on a listener of their own."
specification: "The service SHALL serve its web interface and management API on a listener separate from the webhook listener and the internal listener."
derives_from:
  - "SCEN-014"
---

# System Requirement: Management listener

Accepted in part on 2026-10-18, with increment 1 (SOL-001). Still planned, and not normative until their increment is accepted: the chat-command paths, the Mattermost command callback and the linking links, with increment 5.

## Requirement Specification

> The service SHALL serve its web interface and management API on a listener separate from the webhook listener and the internal listener.

## Rationale

Forges must reach the webhook endpoints, often from outside the organization's network. The management surface should be reachable only by the organization's people. A listener of its own lets platform teams expose each surface differently, for example the management listener only on an internal network.

## Acceptance Criteria

- The webhook listener answers only webhook and chat-command paths.
- The management listener answers the web interface, the API and its OpenAPI description.
- The internal listener keeps probes and metrics (SYSREQ-018, SYSREQ-019).
- Each listener has its own address setting.
- Two startup settings name the listeners' external addresses, which may lie on different hosts. The public base URL belongs to the webhook listener. Forge webhook endpoints and the Mattermost command callback are built from it. The interface URL belongs to the management listener. The OpenID Connect redirect, the setup link and the linking links are built from it. It names only a scheme, a host and a port, because the interface answers at the root of its listener.

## Verification Plan

- **Method**: Test.
- **Procedure**: A test requests every route on every listener and asserts that each answers only on its own.
