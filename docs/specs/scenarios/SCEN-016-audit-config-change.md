---
id: "SCEN-016"
type: scenario
name: "An Instance Admin traces a configuration change"
description: "Announcements changed unexpectedly. The audit log shows who changed which route or template, when, and how."
refines:
  - "UC-004"
---

# Scenario: An Instance Admin traces a configuration change

## Overview

With three roles and an API, configuration changes come from many hands. The audit log answers who did what.

## Initial State

- A route's template was changed, through the API, by a Routing Manager's token.

## Trigger

A team reports that its thread replies look different.

## Step-by-Step Flow

1. The Instance Admin opens the audit log and filters it by the route.
2. The entry names the Routing Manager, the token and the time, and shows the template before and after the change.

## Expected Outcome

- The change is traced to its author without access to the database.

## Exceptions & Edge Cases

- Secret values never appear in entries. Only the fact that a secret changed is recorded.
- Entries cannot be edited or deleted through the web interface or the API.
