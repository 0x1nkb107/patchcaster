---
id: "SCEN-017"
type: scenario
name: "An upgrade imports the file configuration of v0.1"
description: "A v0.1 deployment upgrades to the first database-configured release. Its routing file and environment settings move into the database, and existing threads continue."
status: proposed
refines:
  - "UC-004"
---

# Scenario: An upgrade imports the file configuration of v0.1

## Overview

Existing deployments must keep announcing, in their existing threads, across the change from file configuration to database configuration.

## Initial State

- A v0.1 deployment routes GitLab projects into Mattermost channels through its routing file. Its signing secrets and Mattermost settings come from the environment.
- Its database holds live threads.

## Trigger

The person who runs the deployment upgrades it and runs the import with the old routing file and settings.

## Step-by-Step Flow

1. The import creates a GitLab connection without API access. It keeps the existing signing secrets and answers on the old webhook path, so GitLab's hooks need no change.
2. It creates a source per routing entry, scoped to that entry's project, then the Mattermost connection, a sink per channel and a route per routing entry, all with the built-in templates.
3. It moves the existing thread mappings to their new sinks.
4. It reports what it created.

## Expected Outcome

- The next delivery posts into the same thread as before the upgrade.

## Exceptions & Edge Cases

- The import refuses to run into a configuration that is not empty.
- An invalid routing file aborts the import before anything is written.
