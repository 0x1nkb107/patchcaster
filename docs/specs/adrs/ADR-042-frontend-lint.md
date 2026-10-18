---
id: "ADR-042"
type: architecture_decision_record
name: "ESLint, Prettier and svelte-check guard the web interface"
description: "The web interface's static-analysis gate runs ESLint with typescript-eslint and eslint-plugin-svelte, Prettier with its Svelte plugin, and svelte-check, on a pinned TypeScript 6.0."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-042: ESLint, Prettier and svelte-check guard the web interface

## Context

Go code passes gofmt and the pinned golangci-lint in `just quality` (ADR-011). The web interface is TypeScript and Svelte (ADR-032) and needs a gate of the same weight. On 2026-10-15 the owner chose ESLint and Prettier, with eslint-config-prettier and typescript-eslint. On their own, ESLint and Prettier cannot read `.svelte` files, and the TypeScript compiler cannot type-check them.

## Decision

- ESLint 10 runs with a flat config in `web/`:
  - typescript-eslint's `strictTypeChecked` rules, type-aware through its project service, to match the strict TypeScript of ADR-032.
  - eslint-plugin-svelte's recommended rules, so `.svelte` files are linted too, with typescript-eslint parsing their scripts.
  - eslint-config-prettier comes last and turns off every rule that would contradict Prettier.
- Prettier formats the interface, with prettier-plugin-svelte for `.svelte` files and prettier-plugin-tailwindcss to sort Tailwind classes. Its configuration and ignore file keep it inside `web/`, so it never touches Go code, specs, Markdown docs or YAML.
- svelte-check type-checks the components and treats warnings as errors.
- TypeScript is pinned to 6.0, which is 6.0.3 on 2026-10-15. typescript-eslint 8.71 supports TypeScript below 6.1, svelte-check 4.7 supports 5 and 6, and SvelteKit 3 expects 6. TypeScript 7 waits until all three support it.
- All tools are development dependencies pinned by the lockfile, the frontend's counterpart of the Go tools modules (ADR-007). The lockfile pins ESLint 10.12.0, typescript-eslint 8.71.1, eslint-plugin-svelte 3.23.1, eslint-config-prettier 10.1.8, Prettier 3.9.10, prettier-plugin-svelte 4.1.1, prettier-plugin-tailwindcss 0.8.1 and svelte-check 4.7.6, the newest releases older than seven days on 2026-10-17 (ADR-032). All are MIT licensed.
- Code that tools write stays as they wrote it, so that a new version of the tool can replace it as a whole. ESLint skips the API client that orval generates and the components that shadcn-svelte copies into `src/lib/components/ui`. Prettier skips the generated client.
- `just web-lint` runs `eslint`, `prettier --check` and `svelte-check`, and `just quality` includes it. `just web-fmt` applies Prettier's formatting.

## Consequences

- Good: one gate covers Go and the interface alike, and formatting never comes up in review.
- Good: the type-aware rules catch what the compiler allows, such as unsafe `any` flows and promises nobody awaits.
- Bad: type-aware linting is slower than linting by syntax alone.
- Bad: the TypeScript pin lags the newest release. Each upgrade has to check the supported ranges of all three tools.
