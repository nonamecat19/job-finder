import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { nodePolyfills } from 'vite-plugin-node-polyfills';
import path from 'node:path';

export default defineConfig({
  // memfs (resume preview WASM loader) is a Node fs implementation; it needs
  // Buffer/EventEmitter/path/process shims to run in the browser.
  plugins: [react(), tailwindcss(), nodePolyfills({ include: ['buffer', 'events', 'path', 'process', 'stream', 'util'] })],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, './src') } },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:3000',
    },
  },
});
