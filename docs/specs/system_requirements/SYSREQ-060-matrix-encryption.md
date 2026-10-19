---
id: "SYSREQ-060"
type: system_requirement
name: "Encrypted Matrix rooms"
description: "The bot delivers into end-to-end encrypted rooms as a cross-signed device and keeps its keys in the database."
specification: "The service SHALL deliver into end-to-end encrypted Matrix rooms as a cross-signed device of its bot account, and SHALL keep that device's keys and sessions in its database, encrypted with the instance key, across restarts."
status: proposed
derives_from:
  - "SCEN-028"
depends_on:
  - "SYSREQ-042"
---

# System Requirement: Encrypted Matrix rooms

## Requirement Specification

> The service SHALL deliver into end-to-end encrypted Matrix rooms as a cross-signed device of its bot account, and SHALL keep that device's keys and sessions in its database, encrypted with the instance key, across restarts.

## Rationale

The owner requires encrypted rooms from the start (2026-10-15). An unverified device shows warnings to every room member, and a device that loses its keys after a restart becomes a stranger to the room (ADR-040).

## Acceptance Criteria

- Room members' clients decrypt every message and show it as sent by a verified device.
- The device is cross-signed with the account's recovery key, which an Instance Admin enters once when connecting (SCEN-014).
- After a restart the bot keeps its device id and keys.
- Devices that join a room receive the keys for messages sent after they joined.
- Unencrypted rooms keep working with the same connection.

## Verification Plan

- **Method**: Test.
- **Procedure**: An end-to-end test against a real homeserver container decrypts the bot's messages with a separate test client, before and after a restart of the service.
