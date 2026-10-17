// The interface's lint gate (ADR-042): typescript-eslint's strict
// type-checked rules, eslint-plugin-svelte for .svelte files, and
// eslint-config-prettier last, so that no rule fights Prettier.
import js from '@eslint/js';
import prettier from 'eslint-config-prettier';
import svelte from 'eslint-plugin-svelte';
import { defineConfig } from 'eslint/config';
import globals from 'globals';
import ts from 'typescript-eslint';

export default defineConfig(
	{
		ignores: [
			'.svelte-kit/',
			'dist/',
			'node_modules/',
			// Generated from the API description by orval.
			'src/lib/api/generated/',
			// Vendored by the shadcn-svelte CLI, which overwrites local edits.
			// Prettier and svelte-check still cover them.
			'src/lib/components/ui/'
		]
	},
	js.configs.recommended,
	ts.configs.strictTypeChecked,
	svelte.configs.recommended,
	prettier,
	svelte.configs.prettier,
	{
		languageOptions: {
			globals: { ...globals.browser },
			parserOptions: {
				projectService: true,
				tsconfigRootDir: import.meta.dirname,
				extraFileExtensions: ['.svelte']
			}
		}
	},
	{
		files: ['**/*.svelte', '**/*.svelte.ts'],
		languageOptions: {
			parserOptions: { parser: ts.parser }
		}
	}
);
