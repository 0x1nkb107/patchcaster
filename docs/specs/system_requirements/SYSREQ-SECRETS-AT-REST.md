---
id: "SYSREQ-042"
type: system_requirement
name: "Secrets encrypted at rest"
description: "Every secret in the database is encrypted with the instance key and is never shown again once stored."
specification: "The service SHALL store every secret it holds encrypted with the instance encryption key, and SHALL never return a stored secret through the web interface or the API."
status: proposed
derives_from:
  - "SCEN-014"
depends_on:
  - "SYSREQ-021"
---

# System Requirement: Secrets encrypted at rest

## Requirement Specification

> The service SHALL store every secret it holds encrypted with the instance encryption key, and SHALL never return a stored secret through the web interface or the API.

## Rationale

Moving configuration into the database moves forge tokens, chat credentials and webhook secrets there too. A database dump or backup must not hand them out (ADR-035).

## Acceptance Criteria

- Secrets include forge tokens and OAuth client secrets, chat credentials, webhook secrets, the OpenID Connect client secret, and the Matrix recovery key and encryption state.
- A database dump without the key reveals no secret.
- The interface lets a secret be replaced, never read. The one exception is a new webhook secret, shown once for manual registration (SCEN-014).
- A missing or wrong key stops the service at startup with a message that names the problem, not the key.
- Rotating the key re-encrypts every secret without downtime.

## Verification Plan

- **Method**: Test and inspection.
- **Procedure**: A test scans a dump of the test database for every stored secret's plain value. Tests cover a wrong key and a rotation.
