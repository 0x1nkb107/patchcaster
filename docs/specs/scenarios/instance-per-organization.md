---
id: "SCEN-029"
type: scenario
name: "Each organization runs an instance of its own"
description: "A second organization that wants PatchCaster gets its own instance and database, so no database holds the data of two organizations."
status: proposed
refines:
  - "UC-003"
---

# Scenario: Each organization runs an instance of its own

## Overview

An instance never mixes the data of two organizations.

## Initial State

- An instance serves one organization.

## Trigger

Another organization, such as a subsidiary with an IT department of its own, wants PatchCaster too.

## Step-by-Step Flow

1. A second instance is deployed with its own database, encryption key and public URL.
2. The bootstrap command gives the new organization's IT department a setup link of its own (SCEN-013).

## Expected Outcome

- Each organization's data lives in a database the other does not use.
- Removing an organization means deleting its instance and its database.

## Exceptions & Edge Cases

- The second database may live on a PostgreSQL server that already serves other databases. Only the database itself must be the organization's own.
