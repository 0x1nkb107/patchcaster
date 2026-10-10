---
id: "SYSREQ-026"
type: system_requirement
name: "Deleted threads stay deleted"
description: "A root post removed in Mattermost is treated as an intentional deletion."
specification: "The service SHALL park every Mattermost operation whose stored root post Mattermost reports as missing, treating the absence as an intentional deletion."
derives_from:
  - "SCEN-002"
depends_on:
  - "SYSREQ-006"
---

# System Requirement: Deleted threads stay deleted

## Requirement Specification

> The service SHALL park every reply whose stored root post Mattermost reports as missing, treating the absence as an intentional deletion.

## Rationale

Owner directive (2026-10-10): a root post the bot created, later gone from Mattermost, was
deleted on purpose. Re-posting is the bot arguing with the person who deleted it — every
subsequent merge request event re-opens an argument a human already closed. Treating the absence
as terminal, once, keeps the audit trail honest and the channel calm.

## Acceptance Criteria

- An operation (reply post or root-message update) that Mattermost explicitly reports as
  referring to a missing or deleted root parks that message immediately (not retried), emits one
  structured log record, and increments a parked counter (SYSREQ-019).
- On that first missing-root outcome the thread mapping is marked closed; later events for the
  same merge request are skipped and counted without further Mattermost calls, sharing the
  skip semantics of SYSREQ-007.
- Transient failures (timeouts, `5xx`, `429`) never close a thread; they retry under SYSREQ-016.
  Only an explicit missing-root answer does.

## Verification Plan

- **Method**: Test.
- **Procedure**: Fake Mattermost answering the reply with its missing-root status: assert the
  park, the mapping close, and that a following `update` event is skipped with zero API calls; a
  parallel `500` run asserts retries and an open mapping.

## Notes

The distinction from SYSREQ-007 is origin of knowledge: no-mapping means the bot never had the
thread; mapping-plus-missing-root means the conversation existed and was ended by a human.
Both end in silence, with different counters and different log reasons.
