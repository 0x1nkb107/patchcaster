---
id: "ADR-025"
type: architecture_decision_record
name: "End-to-end tests drive the real binary"
description: "A Go test suite in its own module builds the patchcaster binary and drives it as a separate process against a PostgreSQL container, a fake Mattermost and signed webhooks."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-025: End-to-end tests drive the real binary

## Context

The unit and store tests prove the packages, but nothing exercised the running process. The drain bug fixed in PR #2 lived from the first commit because no test sent a signal to a real process. Several requirements name a process-level test as their verification method: the kill-and-restart test of SYSREQ-017 and the SIGTERM test of SYSREQ-023. On 2026-10-12 the owner asked for these tests in preparation for the first release.

## Decision

- The suite is its own Go module in `e2e/`, like the tool modules. Its test dependencies stay out of the product's `go.mod`, and a root-level `go test ./...` never descends into it, so `just test` stays fast and needs no binary build. `just test-e2e` runs it. The suite imports no product package: it only builds and runs the binary.
- `TestMain` builds the binary once, static and with CGO off as releases are. Each test starts, signals and restarts that binary as a separate process.
- The process runs against a PostgreSQL 18 container that the suite starts for itself through testcontainers-go, a fake Mattermost, and webhooks signed under the Standard Webhooks scheme. Because the database belongs to the suite alone, the tests can recreate its schema without any risk to another database. The fake records the posts it accepts and can fail with 503 or hold a post in flight until the test releases it.
- Each test recreates the database schema and asserts on what operators can observe: the posts that reached Mattermost, the queue and thread rows, the probes, the exit code. Waits poll with a deadline that only guards against a hang.
- The first scenarios cover the thread lifecycle (SCEN-001, SCEN-002), forged and replayed deliveries, the outage retry in order (SCEN-005), a kill after acceptance (SYSREQ-017) and the drain (SYSREQ-023).
- CI runs the suite in its own `e2e` job, on the runner's Docker. `just fmt-check` and `just lint` cover the module too.

## Consequences

- Good: a regression in signal handling, wake-ups, claim order or durability fails CI. Each of the three guards was checked by breaking its behaviour and watching its test fail.
- Good: the suite runs in seconds, because the service takes short poll and lease settings from its flags.
- Bad: the tests need a Docker API, the PostgreSQL image and free local ports. With podman, `DOCKER_HOST` must name podman's socket and testcontainers' cleanup container must be switched off (`TESTCONTAINERS_RYUK_DISABLED=true`), so the suite removes its container itself.
- Bad: testcontainers-go brings a large dependency tree. The separate module keeps it out of the product's `go.mod` and `go.sum`, and so out of the dependency alerts raised for them. It never reaches the binary or its SBOM in either case, because test code is not built into the binary.
- The retry schedule of SYSREQ-016 stays with the unit tests, which control the clock. The end-to-end suite only checks that a retry happens.
- A wake-up test behind a transaction-mode pooler is not yet covered.
