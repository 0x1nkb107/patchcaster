import { execFileSync } from 'node:child_process';

import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

import { thirdParty } from './third-party.ts';

// The commit names the build. SvelteKit would otherwise take the time of
// the build, and the same commit would build a different bundle (ADR-021).
// The browser compares the name with the served one to notice an upgrade.
const commit = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			// A single-page application: every page renders in the browser,
			// and paths that name no file get the fallback page (ADR-032).
			// The bundle goes to dist/app, which web/embed.go embeds into the
			// binary.
			adapter: adapter({
				pages: 'dist/app',
				assets: 'dist/app',
				fallback: 'index.html',
				strict: true
			}),
			version: { name: commit }
		}),
		// Licence notices and an SBOM of the bundled packages (ADR-044).
		thirdParty({
			stylesheet: 'src/app.css',
			notices: 'third-party-licences.txt',
			// The release attaches the SBOM next to the archives. Outside
			// dist, the binary does not embed it.
			sbom: 'sbom/web.cdx.json',
			application: { name: 'patchcaster-web', version: commit },
			// Its licence file grants MIT.
			undeclaredLicences: { 'svelte-toolbelt': 'MIT' },
			// Both declare ISC.
			withoutLicenceFile: ['memoize-weak', 'ts-deepmerge']
		})
	],
	server: {
		// During development, the API and the login come from a local
		// instance's management listener.
		proxy: {
			'/api': 'http://127.0.0.1:8081',
			'/auth': 'http://127.0.0.1:8081'
		}
	}
});
