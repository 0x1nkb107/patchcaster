---
id: "SCEN-022"
type: scenario
name: "Automation manages routes through the API"
description: "A CI job applies a team's routes through the management API with an operator's token, within the operator's permissions."
status: proposed
refines:
  - "UC-005"
---

# Scenario: Automation manages routes through the API

## Overview

The database is the source of truth, but teams that keep configuration in Git still need a way to apply it. The API with its published OpenAPI description is that way.

## Initial State

- An operator created an API token for the team's automation.
- The instance publishes its OpenAPI description.

## Trigger

A CI job applies the routes kept in the team's repository.

## Step-by-Step Flow

1. The job's client, generated from the OpenAPI description, reads the team's current routes.
2. It creates, changes and removes routes until they match the repository.

## Expected Outcome

- Everything the web interface can do, the API can do, with the same permissions.
- Each change appears in the audit log under the operator and the token.

## Exceptions & Edge Cases

- A revoked or expired token answers `401`. A route outside the operator's teams answers `403`.
