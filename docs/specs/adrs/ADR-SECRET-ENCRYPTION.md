---
id: "ADR-035"
type: architecture_decision_record
name: "Secrets are encrypted with an instance key"
description: "Every stored secret is sealed with AES-256-GCM under a key that only the environment holds, bound to the row it belongs to."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-035: Secrets are encrypted with an instance key

## Context

With configuration in the database (ADR-030), forge tokens, chat credentials, webhook secrets, the OpenID Connect client secret and the Matrix keys live there too. Database dumps and backups travel further than a Kubernetes Secret does.

## Decision

- The instance key is 32 random bytes, given base64-encoded through the environment. It is one of the startup settings.
- Each secret is sealed with AES-256-GCM from the standard library, with a random nonce. The additional data names the table, the column and the row, so a sealed value copied into another row fails to open.
- Each sealed value records the id of the key that sealed it. Rotation starts the service with a new key and the old one marked decrypt-only, runs a re-encryption command, and then drops the old key.
- The Matrix pickle key is random and itself sealed like any other secret (ADR-040). Rotating the instance key then re-seals one value instead of every encryption session.
- No surface returns a stored secret. A new webhook secret is shown once for manual registration and never again.

## Consequences

- Good: a dump or backup without the key exposes no credential.
- Good: only the standard library is involved.
- Bad: losing the key loses every stored secret. Connections must then be re-entered, and Matrix devices re-verified.
- Bad: the key is now the one secret deployments must manage carefully.
