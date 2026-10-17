---
id: "ADR-027"
type: architecture_decision_record
name: "The e2e fake Mattermost enforces Mattermost's published API"
description: "The fake Mattermost validates every request against the OpenAPI description of a pinned Mattermost release, so the end-to-end suite fails when PatchCaster calls the API in a way the real server would reject."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-003"
---

# ADR-027: The e2e fake Mattermost enforces Mattermost's published API

## Context

The end-to-end suite of ADR-025 drives PatchCaster against a fake Mattermost. The fake accepted whatever the client sent, so it proved how PatchCaster behaves but not that its requests are valid Mattermost API calls. A renamed field or a missing required property would have passed every test and failed against a real server. Mattermost publishes an OpenAPI description of its REST API v4, assembled from per-area source files in its repository. On 2026-10-13 the owner decided that the fake should check requests against that description.

## Decision

- The suite embeds the endpoints PatchCaster calls, for posts, users and channels, of a pinned Mattermost release, currently v11.11.1 at commit `3acb3a7f684d11ccfcec4e5bd11c79f64e3eabf9`. The script `e2e/testdata/refresh-mattermost-api.sh` holds the pin and assembles `mattermost-openapi-v4.yaml` next to it from `introduction.yaml`, `posts.yaml`, `users.yaml`, `channels.yaml` and `definitions.yaml` at that commit, in the order of Mattermost's own build, under a provenance header. The users and channels parts joined on 2026-10-17, for the connection and sink checks. The pin names a commit because tags can move, and the same commit always gives the same bytes.
- The refresh is a rare maintenance step of the e2e module, not one of the common project tasks of ADR-007. It is therefore no justfile recipe, so the pin, the script and the file it produces stay together in the module that uses them.
- The fake matches every request to an operation of that description and validates its path, parameters and body with kin-openapi. A request that matches no operation, or breaks its schema, is answered with 400, as Mattermost answers it, and recorded. Any recorded breach fails the test when it ends, even if the service went on.
- The description's security requirements are left to the fake's own token check.
- A table test of the check proves it rejects missing required fields, misspelled fields, wrong types and undocumented endpoints, and accepts the calls PatchCaster makes.
- kin-openapi is a dependency of the e2e module only.

## Consequences

- Good: every end-to-end run proves that the root post, the reply and the root refresh are valid calls of the pinned Mattermost API. Renaming the client's `channel_id` field or dropping the update's `id` makes the suite fail with a message that names the operation and the broken property.
- Good: moving to a newer Mattermost release is a reviewed change of two pins and a regenerated file.
- Bad: the check covers requests only. The fake's own answers are not validated against the description, so the client's handling of real Mattermost responses still rests on the client's unit tests.
- Bad: a published description can lag behind or differ from what a server enforces. The check proves conformance to the documented API, not to every server version.
