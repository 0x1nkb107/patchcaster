---
id: "SYSREQ-054"
type: system_requirement
name: "Mentions of linked people"
description: "A template's mention of a forge user becomes a mention of that person's account on the sink's platform, or their plain name."
specification: "The service SHALL render a template's mention of a forge user as a mention of the linked person's account on the sink's platform, and as the user's plain name when no such link exists or the person opted out of mentions."
status: proposed
derives_from:
  - "SCEN-025"
depends_on:
  - "SYSREQ-052"
  - "SYSREQ-053"
---

# System Requirement: Mentions of linked people

## Requirement Specification

> The service SHALL render a template's mention of a forge user as a mention of the linked person's account on the sink's platform, and as the user's plain name when no such link exists or the person opted out of mentions.

## Rationale

The owner decided that identity links enable mentions in threads first (2026-10-15). A mention needs the chain from forge user to person to chat account. Without that chain, a name must stay a name.

## Acceptance Criteria

- On Mattermost a mention is the linked username with an `@`.
- On Matrix a mention is a user link to the linked account, listed in the event's `m.mentions` field so that the person is notified.
- Edits of the root post never notify anyone again.
- Links are resolved when a message is delivered, so a link removed while the message waited is not used.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests render mentions for linked, unlinked and opted-out people on each platform, and assert that root edits carry no new notification.
