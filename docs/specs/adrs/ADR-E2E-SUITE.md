---
id: "ADR-025"
type: architecture_decision_record
name: "End-to-end tests drive the real binary"
description: "A Go test suite behind the e2e build tag builds the patchcaster binary and drives it as a separate process against PostgreSQL, a fake Mattermost and signed webhooks."
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

- The suite lives in the `e2e` package, behind the `e2e` build tag, so `go test ./...` and `just test` stay fast and need no binary build. `just test-e2e` runs it.
- `TestMain` builds the binary once, static and with CGO off as releases are. Each test starts, signals and restarts that binary as a separate process.
- The process runs against a disposable PostgreSQL database, a fake Mattermost and webhooks signed under the Standard Webhooks scheme. The fake records the posts it accepts and can fail with 503 or hold a post in flight until the test releases it.
- Each test recreates the database schema and asserts on what operators can observe: the posts that reached Mattermost, the queue and thread rows, the probes, the exit code. Waits poll with a deadline that only guards against a hang.
- The first scenarios cover the thread lifecycle (SCEN-001, SCEN-002), forged and replayed deliveries, the outage retry in order (SCEN-005), a kill after acceptance (SYSREQ-017) and the drain (SYSREQ-023).
- CI runs the suite in its own `e2e` job with its own PostgreSQL service. golangci-lint builds with the tag, so the suite is linted too.

## Consequences

- Good: a regression in signal handling, wake-ups, claim order or durability fails CI. Each of the three guards was checked by breaking its behaviour and watching its test fail.
- Good: the suite runs in seconds, because the service takes short poll and lease settings from its flags.
- Bad: the tests need PostgreSQL and free local ports, so they cannot run offline.
- The retry schedule of SYSREQ-016 stays with the unit tests, which control the clock. The end-to-end suite only checks that a retry happens.
- A wake-up test behind a transaction-mode pooler is not yet covered.
