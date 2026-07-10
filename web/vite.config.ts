import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	server: {
		// In production the SPA is embedded in and served by craftdeckd
		// itself (same origin). In dev, `npm run dev` runs on 5173 while
		// craftdeckd runs separately on 8080 -- proxy both REST and
		// WebSocket traffic there instead of hardcoding an absolute URL in
		// the frontend code.
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				ws: true
			}
		}
	}
});
