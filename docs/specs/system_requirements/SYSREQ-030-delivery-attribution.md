---
id: "SYSREQ-030"
type: system_requirement
name: "Pipeline and push attribution"
description: "Pipelines attach by linkage, pushes by project and branch; counts appear only when attribution succeeded."
specification: "Pipeline deliveries SHALL update merge request state only when they name that merge request, and push deliveries SHALL be attributed by project and source branch, with push notices stating a commit count only when a matching push delivery was applied at acceptance time."
derives_from:
  - "SCEN-011"
  - "SCEN-012"
depends_on:
  - "SYSREQ-027"
---

# System Requirement: Pipeline and push attribution

## Requirement Specification

> Pipeline deliveries SHALL update merge request state only when they name that merge request,
> and push deliveries SHALL be attributed by project and source branch, with push notices stating
> a commit count only when a matching push delivery was applied at acceptance time.

## Rationale

GitLab fires pipeline hooks for every pipeline, most unrelated to any announced merge request,
and push hooks per repository branch with no merge-request link at all. Guessing attribution
would paint the wrong merge request's board; the owner chose webhook-only derivation over GitLab
read-API calls (ADR-014), accepting honest gaps — most visibly fork pushes, whose push hook fires
on the fork and never reaches this service.

## Acceptance Criteria

- A pipeline delivery carrying a merge-request linkage updates exactly that merge request's
  pipeline state, identified by the global merge request id (the identity rule of SYSREQ-001
  parsing).
- A pipeline delivery without a merge-request linkage is acknowledged with `2xx` and changes no
  state — branch pipelines of an identically named branch do not retroactively attach.
- A push delivery resolves its target by matching repository project and branch reference against
  stored open merge requests' source branches; on match the commit count of the push is stored on
  that merge request's state, on no match it is acknowledged silently.
- A push delivery never posts a notice of its own; the push notice is carried by the merge
  request `update` delivery with `oldrev` (SYSREQ-029), which reads the count stored at
  acceptance if present and otherwise omits the count.
- Attributing one push to several open merge requests sharing a source branch is permitted;
  each notified thread shows the same count.

## Verification Plan

- **Method**: Test.
- **Procedure**: Store contract tests for attribution matching (project + branch, open-state
  requirement, multi-MR branch) and ingestion tests for linked vs unlinked pipeline deliveries,
  asserting state presence/absence and the push-count attach-before-update race both ways
  (push applied first: count present; update accepted first: count omitted).

## Notes

Fork-source merge requests keep a live board (MR, note, and linked pipeline events all arrive
from the target project) but push notices render without a count — the accepted consequence of
staying webhook-only.
