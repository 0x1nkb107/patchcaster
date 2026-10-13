---
id: "ADR-026"
type: architecture_decision_record
name: "Store contract tests start their own PostgreSQL container"
description: "The store contract tests start a disposable PostgreSQL container through testcontainers-go instead of reading a database URL, and fail rather than skip when no Docker API is reachable."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-026: Store contract tests start their own PostgreSQL container

## Context

The store contract tests in `internal/store` read a database URL from `PATCHCASTER_TEST_DATABASE_URL` and recreated the public schema of whatever database it named. Without the variable they skipped, so `just test` passed locally without ever checking the store. `just test-integration` defaulted the variable to the dev database and wiped it. CI ran a PostgreSQL service in two jobs to provide the URL. The end-to-end suite had just moved to a container of its own (ADR-025). On 2026-10-13 the owner decided that the store tests should do the same, and rejected replacing them with a SQL mock such as go-sqlmock, because the contract lives in PostgreSQL's semantics, not in the query text.

## Decision

- `TestMain` in the store tests starts a PostgreSQL 18 container through testcontainers-go and hands its address to the tests. Each test still recreates the schema and migrates through the production path, now in a database that belongs to the test binary alone.
- Without a reachable Docker API the store tests fail with an explicit message. They no longer skip, so a missing database cannot hide the contract.
- `PATCHCASTER_TEST_DATABASE_URL` is gone. `just test` runs the store contract, and `just test-integration` runs it alone.
- CI's `test` and `race` jobs drop their PostgreSQL services and use the runner's Docker.
- testcontainers-go becomes a test dependency of the product module. Adding it upgraded no module that the binary links, which was checked by comparing the binary's module list before and after.
- This amends ADR-006 and ADR-010, which supplied the test database through a URL, and ADR-025, which kept testcontainers out of the product module.

## Consequences

- Good: the store contract runs on every machine that runs the tests, and nothing can point it at a database that matters.
- Good: local runs and CI run the same way, without services or variables.
- Bad: `just test` needs Docker or podman and no longer runs offline. With podman it needs `DOCKER_HOST` and `TESTCONTAINERS_RYUK_DISABLED=true`, as the end-to-end suite does.
- Bad: the product module's `go.mod` and `go.sum` carry the testcontainers dependency tree, so its updates and alerts concern the product module too. Test code never reaches the binary, so the shipped program and its SBOM do not change.
- The end-to-end suite stays a separate module. Its remaining reasons are the black-box boundary, since it imports no product package, and keeping the slow suite out of `go test ./...`.
