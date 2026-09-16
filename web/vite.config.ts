import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// `npm run dev` expects dashd running on 127.0.0.1:7070 (use a mock config).
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../agent/internal/webui/dist',
    emptyOutDir: true,
    assetsInlineLimit: 0,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:7070' },
      '/ws': { target: 'ws://127.0.0.1:7070', ws: true },
    },
  },
});
