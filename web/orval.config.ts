import { defineConfig } from 'orval';

// The interface's API client, its types and its zod schemas come from the
// same description as the server (ADR-031, ADR-032). `just web-api`
// regenerates them; `just web-api-check` fails when they drift.
export default defineConfig({
	client: {
		input: { target: '../internal/api/openapi-v1.yaml' },
		output: {
			target: './src/lib/api/generated/client.ts',
			client: 'fetch',
			mode: 'single',
			baseUrl: '',
			override: {
				mutator: { path: './src/lib/api/fetch.ts', name: 'apiFetch' }
			}
		}
	},
	schemas: {
		input: { target: '../internal/api/openapi-v1.yaml' },
		output: {
			target: './src/lib/api/generated/schemas.ts',
			client: 'zod',
			mode: 'single'
		}
	}
});
