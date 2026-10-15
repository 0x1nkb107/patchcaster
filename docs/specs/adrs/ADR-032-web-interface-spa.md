---
id: "ADR-032"
type: architecture_decision_record
name: "A single-page web interface in strict TypeScript with SvelteKit"
description: "The web interface is a SvelteKit single-page application in strict TypeScript with Bits UI and shadcn-svelte, bundled by Vite, embedded in the binary, and talking only to the management API."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-032: A single-page web interface in strict TypeScript with SvelteKit

## Context

The binary must serve a web interface (UC-004, UC-005, UC-006). Its heaviest screens are interactive: the route editor, and the template editor with a live preview for each platform. On 2026-10-15 the owner chose a single-page application in strongly typed TypeScript, built with SvelteKit, Bits UI and shadcn-svelte and bundled with Vite. PatchCaster ships as one static binary with reproducible release builds (SYSARCH-005, ADR-021).

## Considered Options

- **Server-rendered pages with `html/template`**: one toolchain and reproducible builds without extra work, but interactive screens cost much more effort.
- **A SvelteKit single-page application, chosen by the owner**: rich interaction, types checked from the API contract to the screen, and accessible components, at the price of a second toolchain.

## Decision

- The interface is a SvelteKit application in single-page mode: `@sveltejs/adapter-static` with a fallback page, and server-side rendering switched off in the root layout. It has no server routes. The Go binary is its only backend.
- TypeScript runs in strict mode, and a failed type check fails the build.
- Components come from shadcn-svelte, which adds their source to the repository and builds them on Bits UI. They are styled with Tailwind CSS, which shadcn-svelte requires.
- Vite bundles the application, and the binary embeds the bundle. The layout:
  - `web/` holds the SvelteKit project, with a pinned Node version and an npm lockfile installed by `npm ci`.
  - `web/embed.go` is committed, never generated. It declares the package `web` with `//go:embed all:dist` and an `embed.FS`.
  - `web/dist/.gitkeep` is tracked, so the embed pattern matches before the first frontend build, and Go code compiles on a fresh clone.
  - adapter-static writes the bundle to `web/dist/app` (`pages: 'dist/app'`), which Git ignores. SvelteKit sets Vite's own output directory itself, so the adapter's setting is the one that counts. The adapter clears only its own directory before each build, so the placeholder survives.
  - `go.mod` lists `ignore ./web/node_modules`. npm packages may ship Go files, `flatted` among them, which would otherwise break `go vet ./...`.
- The binary serves `dist/app` on the management listener with `http.FileServerFS`.
  - Paths that name no file and do not start with `/api` get the fallback page.
  - Hashed files under `_app/immutable/` are served with `Cache-Control: public, max-age=31536000, immutable`, and the fallback page with `no-cache`.
  - Without a bundle, the binary serves a short page saying that the interface was not built.
- The interface talks only to the management API. Its types and client are generated from the OpenAPI description (ADR-031). A changed contract then breaks the interface's type check, not its users.
- Login stays on the server (ADR-033). The session lives in an `HttpOnly` cookie on the interface's own origin, so the interface never handles a token.
- `web/.npmrc` sets `min-release-age=7` (owner decision 2026-10-15). When dependencies are added or updated, npm then resolves only versions published more than seven days ago. A hijacked release thus has a week to be noticed and pulled before it can enter the lockfile, and `npm ci` installs the lockfile as reviewed.
  - npm supports the setting from 11.10.0 on. Older releases only warn about an unknown setting and install anyway, so the frontend recipes refuse to run with an npm older than 11.10.0.
  - A fix younger than seven days enters only through `min-release-age-exclude`, one reviewed package at a time.
- Just recipes, CI and the release pipeline build the frontend before the Go binary. The release rehearsal (ADR-023) fails when the bundle is missing, and it builds the bundle twice and compares the results, so reproducibility (ADR-021) covers the bundle too.
- The bundled packages' licence notices ship with the interface, and the interface is served with a strict Content-Security-Policy.

## Consequences

- Good: interactive screens such as the template preview are natural to build, and Bits UI's components take care of keyboard use and accessibility.
- Good: the interface is just another client of the API, so the two can never offer different operations (SYSREQ-035).
- Good: Go code builds and tests without Node, because the placeholder keeps the embed pattern valid. Only the binary that ships needs the frontend built.
- Bad: a second toolchain and its supply chain enter development, CI and every release. The lockfile, `npm ci`, the seven-day release age and an audit gate in CI limit that risk.
- Bad: a security fix published within the last seven days needs an explicit exception before it can be installed.
- Bad: the image SBOM (ADR-022) lists Go modules only. The release must list the bundled JavaScript packages as well, for example in an SBOM of their own.
- Bad: the interface needs JavaScript, which is acceptable for an administration tool.
