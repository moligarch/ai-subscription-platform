import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import sveltePreprocess from 'svelte-preprocess';

export default defineConfig({
  plugins: [
    svelte({
      preprocess: sveltePreprocess(),
    }),
  ],
  server: {
    proxy: {
      '/api': {
        target: 'http://admin.localdev', // Use the new hostname
        changeOrigin: true,
      },
    },
  },
});