import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

const proxy = {
  '/api': {
    target: process.env.API_PROXY || 'http://127.0.0.1:8080',
    changeOrigin: true,
  },
};

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy,
    watch: {
      usePolling: process.env.VITE_USE_POLLING === 'true',
      interval: 300,
    },
    hmr: process.env.HMR_CLIENT_PORT
      ? { clientPort: Number(process.env.HMR_CLIENT_PORT) }
      : undefined,
  },
  preview: { proxy },
});
