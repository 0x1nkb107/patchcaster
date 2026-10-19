---
id: "SCEN-015"
type: scenario
name: "An Instance Admin sets up a team"
description: "An Instance Admin creates a team, gives it a source and sinks, and names its Routing Manager. PatchCaster registers the source's webhook itself."
refines:
  - "UC-004"
---

# Scenario: An Instance Admin sets up a team

## Overview

Teams scope what Routing Managers may route. Instance Admins decide which sources and sinks a team owns.

## Initial State

- A Forgejo connection with API access and a Matrix connection exist (SCEN-014).
- The future Routing Manager has logged in at least once.

## Trigger

The Instance Admin creates the team "Payments".

## Step-by-Step Flow

1. The Instance Admin adds a source: the Forgejo connection with the organization `payments` as its scope.
2. PatchCaster registers a webhook for that organization through the forge API, pointing at the connection's endpoint and signed with the connection's secret.
3. The Instance Admin adds a sink: the Matrix connection and a room the bot was invited to. PatchCaster joins the room and checks it may post.
4. The Instance Admin names the team lead as the team's Routing Manager.

## Expected Outcome

- The Routing Manager sees the team's source and sink and nothing of other teams.
- The audit log records each step.

## Exceptions & Edge Cases

- Without API access, registering the hook is the Instance Admin's manual step, at the connection's endpoint. They then add the signing secret the forge generated for the hook to the connection.
- A source whose scope lies inside an already registered scope, such as one repository of the `payments` organization, needs no hook of its own. The organization hook already delivers its events.
- A sink such as a release channel may be assigned to several teams. Each team's Routing Managers can then route into it.
- Removing a source removes its webhook through the forge API when the connection has API access.
