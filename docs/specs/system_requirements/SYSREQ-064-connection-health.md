---
id: "SYSREQ-064"
type: system_requirement
name: "Connection and sink health"
description: "Each connection and sink reports its health as metrics and in the web interface, and none of them affects readiness."
specification: "The service SHALL expose Prometheus metrics per connection and per sink for deliveries by outcome, for the number of waiting messages and the age of the oldest one, and for the state of every Matrix session and chat command registration, SHALL show the same health in the web interface, and SHALL keep its readiness independent of the health of any connection or sink."
status: proposed
derives_from:
  - "SCEN-009"
  - "SCEN-019"
depends_on:
  - "SYSREQ-018"
  - "SYSREQ-019"
---

# System Requirement: Connection and sink health

## Requirement Specification

> The service SHALL expose Prometheus metrics per connection and per sink for deliveries by outcome, for the number of waiting messages and the age of the oldest one, and for the state of every Matrix session and chat command registration, SHALL show the same health in the web interface, and SHALL keep its readiness independent of the health of any connection or sink.

## Rationale

With many connections, an unreachable homeserver must not stay invisible. It must not take the whole instance out of rotation either, because every other sink still delivers. Readiness therefore keeps the meaning of SYSREQ-018, a writable state store, and the metrics of SYSREQ-019 grow labels for connections and sinks.

## Acceptance Criteria

- Metrics carry connections and sinks as labels by their ids, never by a secret or a personal value. Their number is bounded by the configuration, not by traffic.
- A sink that cannot deliver shows its last error and the time of its last success in the web interface.
- An unreachable forge or chat platform leaves the readiness endpoint unchanged.
- Matrix session state covers the device ownership of SYSREQ-061 and the health of the sync loop. Registration state covers each Mattermost team of SYSREQ-063.

## Verification Plan

- **Method**: Test.
- **Procedure**: An end-to-end test fails one fake sink and asserts its metrics, the health the API reports for the interface, and an unchanged readiness answer.
