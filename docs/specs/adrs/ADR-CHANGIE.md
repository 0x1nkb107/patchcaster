---
id: "ADR-018"
type: architecture_decision_record
name: "changie maintains the changelog"
description: "User-visible changes are recorded as changie fragments and batched per release into a Keep a Changelog file that also supplies the GitHub release notes."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-018: changie maintains the changelog

## Context

Releases (ADR-015) carried release notes generated from commit subjects, grouped by their
`feat`/`fix` prefixes. Commit messages are written for developers: they name ADRs and
internals, and they include changes no operator notices (refactors, CI, tests). The repository
had no changelog file at all. On 2026-10-10 the owner directed that changie
(<https://changie.dev>) maintain and produce the changelog, and that the changelog state it
follows Keep a Changelog 1.1.0 and Semantic Versioning 2.0.0.

## Decision

- **Tool**: changie, pinned as a tools module (`tools/changie`) like every other tool and run
  through `go tool -modfile`; no ambient install.
- **Fragments**: each user-visible change adds one YAML fragment under `.changes/unreleased/`
  (`just change`), written for operators at the time the change is made. Kinds are Keep a
  Changelog's Added, Changed, Deprecated, Removed, Fixed and Security, plus two the owner added:
  **Breaking** for user-facing changes that need action on upgrade, listed first in every
  release, and **Updated** for updated dependencies that ship in the binary or image.
- **Batching**: before a release, `just changelog vX.Y.Z` batches the fragments into
  `.changes/vX.Y.Z.md` and rebuilds `CHANGELOG.md` from the header and every version file. The
  version is named explicitly and must equal the tag about to be cut; changie's automatic
  bumps are not configured, because pre-1.0 a "major" kind must not jump to 1.0.0 on its own.
  A batch without fragments is refused rather than producing an empty release.
- **Format**: `CHANGELOG.md` opens with the Keep a Changelog 1.1.0 and Semantic Versioning
  2.0.0 statement; versions appear newest first as `## [vX.Y.Z](release URL) - YYYY-MM-DD`
  with one `###` section per kind and `-` bullets.
- **Release notes**: `just release` passes the tag's `.changes/<tag>.md` to goreleaser as
  `--release-notes`, replacing the commit-log changelog, and fails before building anything if
  the tag has no batched notes. `changelog.disable` stays unset: goreleaser skips loading the
  notes file when the changelog step is disabled.
- **Distribution**: `CHANGELOG.md` ships in the release archives next to the README.

## Consequences

- Good: operators read notes written for them; the GitHub release body and `CHANGELOG.md` come
  from the same reviewed files.
- Good: one file per change means parallel branches never conflict in the changelog; only the
  batch commit touches `CHANGELOG.md`.
- Bad: cutting a release gains a step — batch, review, commit, then tag (amending ADR-015's
  "tag and push" procedure).
- Bad: whether a change needs a fragment is a review judgement; CI does not enforce it.
- Commit message prefixes no longer shape release notes; they remain a history convention only.

## Alternatives Considered

- **goreleaser's commit-log changelog** (status quo): no maintenance, but developer-facing and
  noisy, and no changelog file.
- **Hand-edited `CHANGELOG.md` with an Unreleased section**: Keep a Changelog's own model, but
  every parallel change edits the same lines and conflicts.
- **Generators from conventional commits** (git-cliff and similar): same audience problem as the
  status quo, since commit messages stay the source.
- **release-please**: drives releases through bot-maintained pull requests; a heavier workflow
  than this single-maintainer project needs, and it would take over tagging from ADR-015's
  pipeline.
