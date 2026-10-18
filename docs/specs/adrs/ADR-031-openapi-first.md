---
id: "ADR-031"
type: architecture_decision_record
name: "The OpenAPI description comes first and generates the API's server code"
description: "The management API is written as an OpenAPI description, from which server interfaces are generated and checked in, like the query code sqlc generates."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-031: The OpenAPI description comes first and generates the API's server code

## Context

The owner wants published OpenAPI specs for the management API (2026-10-15). A description written after the code drifts from it. A description that generates the code cannot.

## Considered Options

- **oapi-codegen, strict server mode**: generates typed handler interfaces for `net/http` and the request and response types. Widely used, with small generated code. Request validation needs a separate middleware built on kin-openapi.
- **ogen, chosen by the owner on 2026-10-15**: generates a statically typed server and client without reflection. Validation and JSON encoding are generated from the description, and tracing and metrics come built in through OpenTelemetry. Version v1.24.0 is Apache-2.0 licensed, which is compatible with AGPL-3.0. In exchange it generates more code, and that code imports ogen's own runtime.
- **Code first**: write handlers and generate the description from them. The description then follows the code, which defeats its purpose as a contract.

## Decision

ogen generates the management API's server and client code.

- The description lives in the repository, uses OpenAPI 3.0, and is embedded in the binary and served by it (SYSREQ-035).
- ogen is pinned as a tools module (ADR-007). The generated code is checked in, and a `just api-check` gate fails CI when it differs from what the description generates, as `just sqlc-check` does for queries (ADR-010).
- Generation turns off ogen's OpenTelemetry feature. API metrics, when they come, use PatchCaster's Prometheus metrics (SYSREQ-019).
- Handlers implement the generated handler interface by calling the configuration service, never the store (SYSARCH-006).
- The generated validation rejects non-conforming requests. Its error handler answers them with `400` problem details that name each violated field by its path (SYSREQ-035, ADR-043).
- The e2e suite uses the generated client, so every end-to-end test also exercises the contract.
- The web interface's TypeScript client and schemas are generated from the same description by orval, and `just web-api-check` checks them the same way (ADR-032).

## Consequences

- Good: the published description and the server can never disagree.
- Good: validation and JSON handling come from the description, without reflection and without a hand-written layer that could drift.
- Good: the typed client serves the e2e suite and a later command-line client alike.
- Bad: the generated code imports ogen's runtime, and that runtime brings more than it needs at run time. Its error package reaches ogen's OpenAPI and JSON Schema packages, and with them zap, two YAML libraries, regexp2 and fatih/color. Its validation package brings shopspring/decimal. All are permissively licensed and compatible with AGPL-3.0. Together they grew the binary by about 1 MiB, 6 %, when the first API landed.
- Bad: API changes start in YAML, which is verbose.
