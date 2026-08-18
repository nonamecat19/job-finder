# job-finder apply helper (Chrome MV3)

Attaches the CV and cover letter job-finder already generated for a vacancy to
that vacancy's apply form on **djinni.co**, **jobs.dou.ua** and **work.ua**.

It never generates documents (that stays in the dashboard) and never submits an
application — it fills the fields and you press Send.

## Build and load

```bash
pnpm --filter @job-finder/shared build     # the extension typechecks against its dist/
pnpm --filter @job-finder/extension build  # -> apps/extension/dist
```

Then `chrome://extensions` → Developer mode → **Load unpacked** →
`apps/extension/dist`. Rebuilding requires hitting the reload button on the
extension card; there is no HMR for content scripts.

## Configuration

The options page (Settings in the popup, or the extension's Details → Extension
options) holds the API base URL, default `http://localhost:3000`. Pointing it at
another origin triggers a Chrome permission prompt for that origin, because the
manifest only ships host permissions for localhost.

## How it works

- **All API access lives in the service worker** (`src/background/api.ts` is the
  only module that calls `fetch`, enforced by an eslint rule). The vacancy pages
  are https and the API is http; only an extension context can reach it, and
  keeping the content script API-free means a job board can never talk to your
  local job-finder through this extension.
- The content script receives only the resolved bytes and text for the one
  document you picked — never a URL, an id, or a list.
- The tab URL is matched to a job through `GET /api/jobs?url=` (exact), with a
  title search as a fallback. A vacancy that isn't in job-finder is reported as
  such; adding it is an explicit button, because `POST /api/jobs/manual` scrapes
  and writes.

## Manual verification

Preconditions: infra up (`make up`) and the Go API on `:3000`; a vacancy in
job-finder with a `resume` document whose PDF is rendered and a `cover_letter`.

1. Build and load unpacked as above.
2. Options page shows `http://localhost:3000` → **Test connection** → "Connected".
3. Open the vacancy on djinni.co (logged in) and click the toolbar icon. The
   popup shows the vacancy title and company **from job-finder**, plus a CV row
   and a cover-letter row with version and date.
4. If the apply form is closed, click **Open apply form**; the buttons enable.
5. **Attach CV** → the site's file widget shows the PDF filename. With the
   page's DevTools → Network open, confirm no request to `localhost:3000`
   originated from the page.
6. **Paste letter** → the textarea fills *and* the site's own character counter
   or validation updates (that is the proof the React-safe setter worked).
7. Nothing is submitted. Press Send yourself to complete the application.
8. Repeat on jobs.dou.ua and work.ua.
9. Negative cases: a vacancy not in job-finder → "isn't in job-finder yet" plus
   **Add**; a document with no rendered PDF → "PDF hasn't been rendered yet";
   the API stopped → "Can't reach job-finder at http://localhost:3000".

## Tests

`make test-extension` (vitest + jsdom). The adapter fixtures under
`tests/fixtures/` are **synthetic** — see the README there — so they prove the
adapter contract, not that the primary selectors match production markup. Replace
them with real captures the first time this is driven against a live vacancy.
