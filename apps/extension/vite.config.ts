import react from '@vitejs/plugin-react';
import { defineConfig, type Plugin } from 'vite';
import fs from 'node:fs';
import path from 'node:path';

const OUT_DIR = path.resolve(import.meta.dirname, 'dist');

function flattenHtmlEntries(): Plugin {
  return {
    name: 'jobfinder-flatten-html-entries',
    closeBundle() {
      for (const entry of ['popup', 'options']) {
        const from = path.join(OUT_DIR, 'src', entry, `${entry}.html`);
        if (!fs.existsSync(from)) continue;

        fs.renameSync(from, path.join(OUT_DIR, `${entry}.html`));
      }
      fs.rmSync(path.join(OUT_DIR, 'src'), { recursive: true, force: true });
    },
  };
}

export default defineConfig({
  plugins: [react(), flattenHtmlEntries()],
  publicDir: 'public',
  resolve: { alias: { '@': path.resolve(import.meta.dirname, './src') } },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'chrome116',
    sourcemap: true,
    rollupOptions: {
      input: {
        popup: path.resolve(import.meta.dirname, 'src/popup/popup.html'),
        options: path.resolve(import.meta.dirname, 'src/options/options.html'),
        background: path.resolve(import.meta.dirname, 'src/background/index.ts'),
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: 'chunks/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
});
