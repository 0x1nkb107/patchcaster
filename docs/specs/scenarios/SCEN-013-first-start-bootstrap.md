---
id: "SCEN-013"
type: scenario
name: "The first start bootstraps the first administrator"
description: "A fresh instance has no configuration and no administrator. A one-time setup link leads to the identity provider setup and the first administrator."
status: proposed
refines:
  - "UC-004"
---

# Scenario: The first start bootstraps the first administrator

## Overview

A new instance must be reachable by its first administrator without any default password, and the way in must close once it was used.

## Initial State

- The instance runs with its database URL, encryption key, public base URL and interface URL. The configuration in the database is empty.

## Trigger

The person who deployed the instance runs the bootstrap command.

## Step-by-Step Flow

1. The command prints a setup link that works once and expires after a short time.
2. The person opens the link and enters the OpenID Connect issuer, client id and client secret. Optionally, they name the groups allowed to log in.
3. PatchCaster sends them through the identity provider's login. The identity that comes back becomes the first administrator.
4. The setup link stops working.

## Expected Outcome

- One administrator exists, and later logins go through the identity provider.
- The setup path is closed for good, and the audit log records the bootstrap.

## Exceptions & Edge Cases

- An expired or used setup link is refused. Running the command again issues a new one while no administrator exists.
- If the identity provider settings are wrong, the setup stays open and explains the error. Nothing is half-saved.
- If every administrator lost access, for example after an identity provider change, the recovery command grants the administrator role to a named identity. It needs the database credentials and leaves an audit entry.
