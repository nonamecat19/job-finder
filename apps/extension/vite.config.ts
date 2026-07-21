import { defineConfig } from 'vite'
import { crx } from '@crxjs/vite-plugin'

const manifest = {
  manifest_version: 3,
  name: 'Job Finder Autofill',
  version: '0.1.0',
  description: 'Autofill job applications with your stored profile',
  permissions: ['storage'],
  host_permissions: [
    'http://localhost/*',
    'https://example.com/*',
  ],
  content_scripts: [
    {
      matches: [
        'http://localhost/*',
        'https://example.com/*',
      ],
      js: ['src/content-script.ts'],
      run_at: 'document_idle',
    },
  ],
  background: {
    service_worker: 'src/service-worker.ts',
    type: 'module',
  },
}

export default defineConfig({
  plugins: [crx({ manifest })],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
