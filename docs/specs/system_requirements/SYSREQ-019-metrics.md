---
id: "SYSREQ-019"
type: system_requirement
name: "Delivery metrics"
description: "Prometheus counters cover the ingest-to-post funnel."
specification: "The service SHALL expose Prometheus metrics counting webhook deliveries received, rejected, suppressed, and posted, merge request state applications and notices by kind and cause, push attributions missed, with Mattermost delivery attempt counts, each labelled by its connection and sink, and gauges of every sink's waiting messages."
derives_from:
  - "SCEN-009"
---

# System Requirement: Delivery metrics

## Requirement Specification

> The service SHALL expose Prometheus metrics counting webhook deliveries received, rejected, suppressed, and posted, with Mattermost delivery attempt counts, each labelled by its connection and sink, and gauges of every sink's waiting messages.

## Rationale

The infrastructure team diagnoses with the funnel: received, rejected, suppressed, ignored, posted, and at what
cost. The same counters answer the solution KPIs (loss and latency) and alert on backlog growth.

## Acceptance Criteria

- The metrics endpoint exposes counters for deliveries received, rejected by cause (signature,
  parse), suppressed duplicates, discarded (expired, unrouted, non-allowlisted action, or
  non-merge-request event — SCEN-003), skipped (no thread mapping, thread closed, or duplicate
  root redelivery — SYSREQ-007/SYSREQ-026), parked messages, messages posted,
  and Mattermost attempts by outcome; plus a delivery attempt latency histogram.
- Ingest counters carry the forge connection, and delivery counters the chat connection and the sink, as labels by their ids. Two gauges show each sink's waiting messages and the age of the oldest (SYSREQ-064).
- Counter names share one `patchcaster_` prefix *(names assumed)* and render in Prometheus text
  format on the internal listener.

## Verification Plan

- **Method**: Test.
- **Procedure**: Drive handler scenarios (accept, reject, suppress, ignore, post) against the
  registry, scrape the endpoint, and assert exact deltas.

## Notes

Amended on 2026-10-18 by SYSREQ-064: the counters carry connection and sink labels, and the waiting gauges join them.
