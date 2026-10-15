---
id: "SYSREQ-059"
type: system_requirement
name: "Matrix threads and edits"
description: "In a Matrix room the root is a message, later activity replies in its thread, and board refreshes edit the root."
specification: "The service SHALL post a merge request's root into a Matrix room as a message, its later activity as replies in that message's thread, and its board refreshes as edits of that message."
status: proposed
derives_from:
  - "SCEN-028"
---

# System Requirement: Matrix threads and edits

## Requirement Specification

> The service SHALL post a merge request's root into a Matrix room as a message, its later activity as replies in that message's thread, and its board refreshes as edits of that message.

## Rationale

Matrix threads, through the `m.thread` relation, and edits, through the `m.replace` relation, match what the Mattermost delivery does with threads and post updates.

## Acceptance Criteria

- Replies carry the `m.thread` relation to the root event.
- Board refreshes are `m.replace` edits of the root, carrying the complete new content.
- Rate-limit answers are honoured with the wait the homeserver names, and count as retries.

## Verification Plan

- **Method**: Test.
- **Procedure**: An end-to-end test against a real homeserver container asserts the relations of every event the bot sends.
