import { defineConfig } from 'vite'
import { crx } from '@crxjs/vite-plugin'

// ATS domains the content script is allowed to run on (spec 014-autofill-
// extension 1.1/6: host permissions "scoped narrowly per detected ATS
// domain and never include <all_urls>"). Adding a new ATS requires a
// manifest update here, matching src/ats.ts's detectAts().
const ATS_MATCHES = [
  'https://boards.greenhouse.io/*',
  'https://job-boards.greenhouse.io/*',
  'https://jobs.lever.co/*',
  'https://*.myworkdayjobs.com/*',
]

// The job-finder API origin (see src/config.ts — must match API_BASE_URL).
// Needed so the service worker can call /ext/auth/* and /ext/profile.
const API_ORIGIN_MATCH = 'http://localhost:3000/*'

const manifest = {
  manifest_version: 3,
  name: 'Job Finder Autofill',
  version: '0.1.0',
  description: 'Autofill job applications with your stored profile',
  // 'storage' only — the content script is declared statically above (never
  // injected programmatically), so no 'scripting'/'activeTab' is needed;
  // chrome.tabs.query({active,currentWindow}) and chrome.tabs.sendMessage
  // (popup.ts) both work without extra permission when only the tab id is
  // read.
  permissions: ['storage'],
  host_permissions: [...ATS_MATCHES, API_ORIGIN_MATCH],
  content_scripts: [
    {
      matches: ATS_MATCHES,
      js: ['src/content-script.ts'],
      run_at: 'document_idle',
    },
  ],
  background: {
    service_worker: 'src/service-worker.ts',
    type: 'module',
  },
  action: {
    default_popup: 'src/popup/index.html',
    default_title: 'Job Finder Autofill',
  },
}

export default defineConfig({
  plugins: [crx({ manifest })],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
