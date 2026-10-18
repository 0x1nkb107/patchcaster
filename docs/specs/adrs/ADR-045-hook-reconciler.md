---
id: "ADR-045"
type: architecture_decision_record
name: "A background reconciler registers the forge hooks of the sources"
description: "PatchCaster registers the hooks of a forge connection's sources in the background: it compares the hooks the sources need with the ones it registered, changes the forge to match, and records the forge's answer for each source."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-007"
---

# ADR-045: A background reconciler registers the forge hooks of the sources

## Context

SYSREQ-044 requires PatchCaster to register, through the forge API, one hook for each source scope that no broader scope covers. GitLab offers three kinds of hooks, each with its own limits:

- A project hook needs the project's Maintainer role.
- A group hook covers the group's projects and subgroups, but needs GitLab Premium and the group's Owner role.
- A system hook covers the instance, but needs an administrator, and it delivers merge request and push events only: no comments and no pipelines.

Since GitLab 19.0, all three accept a signing token in the form `whsec_` plus a base64 key, and GitLab never shows it again. A forge can be slow, down or refuse a hook, and several replicas of PatchCaster may run at once. On 2026-10-18 the owner decided to register hooks in the background, to serve an instance source with a system hook plus narrower hooks for comments and pipelines, and to show GitLab's refusal of a group hook instead of falling back to project hooks.

## Considered Options

- **Register within the API request**: immediate feedback, but a forge outage blocks configuration, and a removal that fails halfway leaves a hook behind.
- **A background reconciler, chosen by the owner**: configuration never waits for the forge, and every failure is retried.
- For an instance source: let the system hook replace every narrower hook, which loses comments and pipelines everywhere, or never register it. The owner chose the system hook with narrower hooks for the events it lacks.
- For GitLab Free: hook every project of a group one by one and rescan for new projects. The owner chose to show GitLab's refusal instead.

## Decision

- A reconciler in package `hooks` derives the hooks the sources of a forge connection need, compares them with the hooks recorded in table `forge_hooks`, and changes the forge to match. It registers new hooks and updates changed ones first, and removes hooks last, so that no event falls into a gap while a broader hook replaces narrower ones.
- An instance source gets a system hook for merge requests and pushes. The narrower scopes that no broader group covers then keep hooks for comments and pipelines alone, so that no event arrives twice (owner decision 2026-10-18). Without an instance source, they take all four triggers.
- A change of a source pokes the reconciler, and so does a change of a forge connection. Besides, it passes over every forge connection at start and every five minutes. A pass retries what failed, and follows a changed public URL by updating the hooks' address.
- One replica at a time works on a connection, under a transaction-scoped PostgreSQL advisory lock. A replica that finds the connection taken tries again ten seconds later, and a replica that dies frees the lock with its session.
- Each hook gets a signing token of its own: 32 random bytes in GitLab's form. PatchCaster sends it with the creation, and stores it sealed among the connection's signing tokens once GitLab accepted the hook, in one audited transaction. When storing fails, PatchCaster removes the hook again. The token and the connection cannot be removed while the hook's record remains.
- PatchCaster names its hooks "PatchCaster". Before it registers a hook at a scope, it removes the hooks there that carry that name and the connection's address but no record, which an interrupted registration may have left. It never touches a hook registered by hand.
- A hook that the forge reports missing on an update is registered anew. A hook that the forge reports missing on removal is forgotten.
- A failed attempt is recorded with the forge's answer. Each source shows how its events arrive: registered, pending, failed with the answer, covered with the broader scope, or manual for a connection without API access. The state is a read-only part of the source, outside its version (ADR-043).
- Without API access, nothing changes at the forge. Registered hooks stay until a token allows their removal.

## Consequences

- Good: configuration never waits for the forge, and a refused or failed hook is retried and shows why.
- Good: several replicas can run without registering a hook twice.
- Bad: a change reaches the forge a moment after the API answered, and up to five minutes later after a failure.
- Bad: PatchCaster does not poll the forge for its hooks. A hook removed by hand is noticed at the next change of that hook. Until then the forge connection's health shows no new deliveries (SYSREQ-064).
- Bad: a repository that only an instance source covers gets no comments and no pipelines, because a system hook does not deliver them.
- Bad: a crash between the forge's creation of a hook and its record leaves a hook whose token is lost. Its deliveries fail verification, which the forge connection's health shows, until the next registration at that scope removes it.
