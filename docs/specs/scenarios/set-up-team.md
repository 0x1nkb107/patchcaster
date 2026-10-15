---
id: "SCEN-015"
type: scenario
name: "An administrator sets up a team"
description: "An administrator creates a team, gives it a source and sinks, and names its operator. PatchCaster registers the source's webhook itself."
status: proposed
refines:
  - "UC-004"
---

# Scenario: An administrator sets up a team

## Overview

Teams scope what operators may route. Administrators decide which sources and sinks a team owns.

## Initial State

- A Forgejo connection with API access and a Matrix connection exist (SCEN-014).
- The future operator has logged in at least once.

## Trigger

The administrator creates the team "Payments".

## Step-by-Step Flow

1. The administrator adds a source: the Forgejo connection with the organization `payments` as its scope.
2. PatchCaster registers a webhook for that organization through the forge API, pointing at the connection's endpoint and signed with the connection's secret.
3. The administrator adds a sink: the Matrix connection and a room the bot was invited to. PatchCaster joins the room and checks it may post.
4. The administrator names the team lead as the team's operator.

## Expected Outcome

- The operator sees the team's source and sink and nothing of other teams.
- The audit log records each step.

## Exceptions & Edge Cases

- Without API access, registering the hook is the administrator's manual step, with the endpoint and secret the connection showed when it was created.
- A source whose scope lies inside an already registered scope, such as one repository of the `payments` organization, needs no hook of its own. The organization hook already delivers its events.
- A sink such as a release channel may be assigned to several teams. Each team's operators can then route into it.
- Removing a source removes its webhook through the forge API when the connection has API access.
