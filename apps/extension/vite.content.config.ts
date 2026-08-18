import { defineConfig } from 'vite';
import path from 'node:path';

export default defineConfig({
  publicDir: false,
  resolve: { alias: { '@': path.resolve(import.meta.dirname, './src') } },
  build: {
    outDir: 'dist',
    emptyOutDir: false,
    target: 'chrome116',
    sourcemap: true,
    lib: {
      entry: path.resolve(import.meta.dirname, 'src/content/index.ts'),
      formats: ['iife'],
      name: 'JobFinderContent',
      fileName: () => 'content.js',
    },
    rollupOptions: { output: { inlineDynamicImports: true, extend: true } },
  },
});
