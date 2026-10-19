---
id: "ADR-043"
type: architecture_decision_record
name: "Conventions of the management API"
description: "The management API versions in its path, reports errors as problem details with field paths that form libraries understand, pages with cursors, and refuses conflicting writes."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-043: Conventions of the management API

## Context

ADR-031 makes the OpenAPI description the source of the management API. Its ground rules must be fixed before the first description is written. Automation and a later Terraform provider will depend on the API, and the web interface's forms must show the API's errors next to the fields they concern. On 2026-10-16 the owner decided that the API versions in its path, and that its errors fit frontend form tooling such as formsnap.

## Decision

- **Versioning:** every operation lives under `/api/v1/`, and each version has its own OpenAPI description. Within a version, changes are only additive: new operations, new optional request fields and new response fields. A change that would break a client opens `/api/v2/`, which runs beside `/api/v1/` for at least one release.
- **Errors:** every error answers with `application/problem+json` as defined by RFC 9457, with `type`, `title`, `status` and `detail`.
  - Validation errors carry an `errors` member: a list of entries, each with a `path` and its `messages`.
  - A path names the request field the way JavaScript form libraries do, for example `routes[0].sink_id`. An empty path means the request as a whole.
  - This is the shape that sveltekit-superforms reports as all of a form's errors. The interface can therefore attach each message to its field with superforms' `setError`, without mapping code per form.
  - The interface uses sveltekit-superforms 3 without formsnap (owner decision 2026-10-17). formsnap 2.0.1 needs superforms 2, which supports SvelteKit 2 at most, and the interface builds on SvelteKit 3 (ADR-032). A small field component of the interface's own shows each field's label and messages instead.
- **Names:** JSON members use snake_case, as PatchCaster's stored events already do.
- **Lists:** lists page with a `limit` and an opaque `cursor`. Each page carries the cursor of the next one, and the order is stable.
- **Concurrent changes:** every resource carries an `ETag`. A change must send `If-Match` with it, and it fails with `412 Precondition Failed` when the resource changed meanwhile. A change without `If-Match` answers `428 Precondition Required`. The interface and automation therefore cannot overwrite each other unnoticed. Revocations are the exception: repeating one changes nothing and no concurrent change can be lost, so they need no `If-Match`, and API tokens carry no `ETag` (owner decision 2026-10-17).
- **Runtime state:** state that changes with every delivery, such as the health of a connection or sink (SYSREQ-064), is a read-only part of its resource. The `ETag` covers the configuration alone, so a change never fails with `412` because a delivery happened meanwhile (owner decision 2026-10-18).
- **Authentication:** the interface uses its session cookie, and automation uses `Authorization: Bearer` with an API token (SYSREQ-040). Both reach the same authorizer (SYSREQ-039).

## Consequences

- Good: clients can rely on `/api/v1/` until they choose to move on.
- Good: the interface shows the server's validation errors next to the right fields with one shared helper.
- Good: no change between the interface and automation is lost silently.
- Bad: every change needs the current `ETag`, so clients read before they write.
- Bad: a breaking change costs a second version of the API for a while.
