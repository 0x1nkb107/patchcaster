---
id: "SYSREQ-035"
type: system_requirement
name: "Management API with an OpenAPI description"
description: "Every configuration operation of the web interface is available through an HTTP API, which the service describes in OpenAPI."
specification: "The service SHALL offer every configuration operation of its web interface through an HTTP API, SHALL publish that API's OpenAPI description at a fixed path, and SHALL answer requests that do not conform to the description with HTTP 400."
status: proposed
derives_from:
  - "SCEN-022"
---

# System Requirement: Management API with an OpenAPI description

## Requirement Specification

> The service SHALL offer every configuration operation of its web interface through an HTTP API, SHALL publish that API's OpenAPI description at a fixed path, and SHALL answer requests that do not conform to the description with HTTP 400.

## Rationale

Teams that keep configuration in Git, and later integrations such as a command-line client or a Terraform provider, need a stable contract. The owner asked for published OpenAPI specs (2026-10-15). A description generated from the same source as the server cannot drift from it (ADR-031).

## Acceptance Criteria

- Each operation of the web interface has an API operation with the same effect and the same authorization (SYSREQ-039).
- The description the service publishes is the one its server code was generated from.
- A request that breaks the description answers `400` with problem details that name each violated field by its path, and has no side effect.
- Operations live under `/api/v1/`, and a change that would break clients opens a new version path (ADR-043).

## Verification Plan

- **Method**: Test and inspection.
- **Procedure**: Contract tests validate requests and responses against the published description. A CI check fails when the committed generated code differs from what the description generates.
