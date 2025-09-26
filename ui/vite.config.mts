// vite.config.mts
import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import sveltePreprocess from 'svelte-preprocess';

// Dev proxy sends /api/* from the Vite dev server (localhost) to your live backend.
// This matches your Dockerized setup where the app is only reachable via Caddy.
export default defineConfig({
  plugins: [
    svelte({
      preprocess: sveltePreprocess(),
    }),
  ],
  server: {
    host: true, // allow LAN access if needed
    proxy: {
      '/api': {
        target: 'https://admin.sibgpt.app',
        changeOrigin: true,
        secure: true, // verify TLS cert (Let's Encrypt should be fine)
        // No path rewrite needed; /api/* is preserved by Caddy and your backend
      },
    },
  },
});