---
id: "ADR-006"
type: architecture_decision_record
name: "testify with arrange-act-assert"
description: "All tests use stretchr/testify assertions following the arrange-act-assert pattern."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# Architecture Decision: testify with arrange-act-assert

## Context and problem statement

Every requirement carries a test-based verification plan; the project needs one consistent test
idiom so a failing test reads the same everywhere.

## Key factors

- Owner directive: tests use stretchr/testify following arrange-act-assert.
- Table-driven tests remain the Go idiom for parameterized checks (repository policy).
- Determinism: injected clocks, `httptest` doubles, and a disposable PostgreSQL database via a
  test connection URL — no external network or wall-clock reliance. (Amended by ADR-026: the store tests now start their own PostgreSQL container instead of reading a URL.)

## Considered options

- testify (`assert`/`require`) with AAA structure.
- Bare `testing` with hand-written failure strings.
- BDD frameworks (ginkgo/gomega).

## Decision Outcome

Chosen option: "testify with arrange-act-assert", by owner directive; `require` guards
preconditions, `assert` collects outcome checks, and the AAA blocks are separated by blank lines
so the reader sees set-up, action, and expectation as a triad.

### Positive Consequences

- Consistent failure output across the suite; less boilerplate than hand-rolled comparisons.
- Mocks stay plain Go fakes behind the narrow interfaces of SYSARCH-001/002/003.

### Negative Consequences

- One test dependency; acceptable at test scope only (never imported by non-test code).

## Pros and Cons of the Options

### testify + AAA

- Good, because assertion messages read as requirements.
- Bad, because assert/require confusion can mask precondition failures — lint rule prefers
  `require` until the first act.

### bare testing

- Good, because zero dependencies; bad, because every suite reinvents its failure text.

## Links

- Justifies SYSARCH-005 (verification harness for all components).
