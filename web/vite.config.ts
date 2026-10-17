import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

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
			})
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
