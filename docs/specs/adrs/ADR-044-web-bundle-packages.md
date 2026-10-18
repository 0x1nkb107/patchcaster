---
id: "ADR-044"
type: architecture_decision_record
name: "The web bundle lists its packages for licence notices and an SBOM"
description: "A Vite plugin of the project lists the npm packages whose code the browser bundle holds. The interface serves their licence notices, and the release attaches a CycloneDX SBOM of them that the signed checksums cover."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
  - "SYSARCH-006"
---

# ADR-044: The web bundle lists its packages for licence notices and an SBOM

## Context

The binary embeds the web interface's bundle (ADR-032), and with it code from npm packages under MIT and ISC licences. Both licences require their notice in every copy. The image SBOM of ADR-022 lists the Go modules and the base image, but not the JavaScript packages inside the binary. ADR-032 therefore requires licence notices with the interface and a list of the bundled packages in the release.

The lockfile names about 380 packages, most of them build tools. Only a few dozen reach the bundle, and the bundler alone knows which: it drops code that nothing uses. On 2026-10-17 the owner chose to derive the list from the bundler with a plugin of the project's own, and to attach the SBOM to the release only.

## Considered Options

- **A Vite plugin of the project's own, chosen by the owner**: it reads which modules the browser build holds and writes both outputs itself. No new dependency, and the outputs stay reproducible.
- **rollup-plugin-license and `npm sbom`**: two existing tools. `npm sbom` lists every production dependency in the lockfile, so the packages would have to be split into dependencies and development dependencies. It would still list SvelteKit's server-side dependencies, which never reach the browser. Its output carries a timestamp and a random serial number, which would have to be stripped for reproducibility. rollup-plugin-license was untested with the Rolldown bundler of Vite 8.

## Decision

- `web/third-party.ts` is a Vite plugin that runs in the browser build only. It collects the packages of every module that contributes code to the bundle:
  - modules below `node_modules`, by the package directory they belong to;
  - the virtual modules that Rolldown and Vite fill with their own runtime code, as the `rolldown` and `vite` packages. A virtual module of any other origin fails the build;
  - the packages that `src/app.css` imports by name. Tailwind inlines them, so the bundler never sees them as modules.
- The bundle holds the notices as `third-party-licences.txt`, and every page links to it. Each entry names a package, its version and its licence, followed by the licence files the package ships.
- Exceptions are explicit in `web/vite.config.ts`, and the build fails on any package that needs one but is not listed:
  - `undeclaredLicences` names the licence of a package whose manifest declares none, as its licence file grants it. svelte-toolbelt is listed, under MIT.
  - `withoutLicenceFile` lists packages that ship no licence file. Their notice holds the standard text of the licence they declare, with their author as the copyright holder. memoize-weak and ts-deepmerge are listed, both under ISC.
  - An exception that no longer applies fails the build too, so that the list never goes stale.
- The plugin writes a CycloneDX 1.6 SBOM to `web/sbom/web.cdx.json`. It lies outside `web/dist`, so the binary does not embed it.
  - Each component has its name, scope, version, licence expression, package URL, the lockfile's integrity hash and the address it was downloaded from. The document's metadata names the commit.
  - The document has no timestamp and no serial number. CycloneDX makes both optional, and without them the same lockfile writes the same bytes (ADR-021).
- The release attaches the SBOM as `patchcaster_<version>_web.cdx.json`. goreleaser's SBOM step copies it into the release. `checksums.txt` lists it, so the cosign signature of ADR-016 covers it. The image does not carry it (owner decision 2026-10-17).
- The release rehearsal (ADR-023) checks that `checksums.txt` lists the SBOM, that the SBOM names the commit and lists the bundled packages, and that a second install and build writes the same SBOM.
- This amends ADR-022, which says that the release ships no SBOM beside the image attestations. The archives still carry none, but the release attaches this one. The objection of ADR-022, timestamps and random identifiers that break reproducibility, does not apply to it.

## Consequences

- Good: the notices and the SBOM follow the code that ships. A package that the bundler drops is not listed.
- Good: no new dependency, and both outputs are reproducible.
- Bad: the plugin is code to maintain. It relies on the module information of Rolldown's chunks and on the names of Vite's virtual modules.
- Bad: a new package without licence data, or a new virtual module, fails the build until someone reviews it.
- The image's SBOM attestation still lists only the base image and the Go modules. The JavaScript packages are in a separate document.
- This covers the web bundle only. The Go modules linked into the binary carry licences that require notices too.
