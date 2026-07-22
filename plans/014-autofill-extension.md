# Plan: 014 Autofill Extension — Auth + Fill UI

**Status**: Detection core done, auth + fill UI missing. The field detection engine (ATS identification, form field detection, field maps, heuristics) is well-implemented with tests, but the extension can't authenticate or fill forms.

**Spec**: `specs/014-autofill-extension/spec.md`

## What Exists

| Layer | Status |
|-------|--------|
| Extension structure (`apps/extension/`) | Done — package.json, vite.config.ts, tsconfig.json |
| ATS detection (`src/ats.ts`) | Done |
| Form field detection (`src/detect.ts`, 103 lines) | Done |
| Field maps (`src/maps.ts`, 53 lines) | Done |
| Heuristics (`src/heuristics.ts`, 37 lines) | Done |
| Test fixtures (4 HTML files) | Done |
| Auth (bootstrap code, token exchange) | **Missing** |
| Profile fetch from API | **Missing** |
| Content script fill logic | **Missing** (only console.log) |
| Service worker | **Missing** (only console.log) |
| Popup UI | **Missing** |
| Fill UI (badge, overlay, confirmation) | **Missing** |

## Tasks

### 1. Backend: Extension auth endpoints

- [ ] **1.1** `POST /api/v1/ext/auth/exchange` in `apps/api`:
  - Accept bootstrap code (short-lived, 5 min TTL)
  - Return access token (JWT, 1h TTL, scope: `profile:read`) + refresh token (opaque, 30d TTL)
- [ ] **1.2** `POST /api/v1/ext/auth/refresh`:
  - Accept refresh token, rotate (one-time-use)
  - Return new access + refresh token pair
- [ ] **1.3** `GET /api/v1/ext/profile`:
  - Return authenticated user's profile fields (fullName, email, phone, location, headline, skills, workHistory, education, links)
  - Enforce `profile:read` scope
  - Reject cross-user access (sub claim must match profile owner)
- [ ] **1.4** Dashboard: Generate bootstrap code UI (QR + string, 5 min TTL)
- [ ] **1.5** Add `BootstrapCode` table or in-memory store with TTL

### 2. Extension: Service worker

- [ ] **2.1** `src/service-worker.ts`:
  - On install: check for existing tokens
  - Token refresh lifecycle (check session → refresh if needed → re-auth if expired)
  - Message relay: content script requests → fetch from API → return profile data
  - Store access token in `chrome.storage.session`, refresh token in `chrome.storage.local`
- [ ] **2.2** `src/auth.ts`:
  - Bootstrap code exchange flow
  - Token storage and retrieval helpers
  - Auto-refresh before expiry

### 3. Extension: Content script fill logic

- [ ] **3.1** `src/content-script.ts`:
  - On page load: detect ATS, scan form fields
  - Send detected fields to service worker for matching
  - Receive matched profile values
  - Show "fields ready to fill" indicator (badge or overlay)
  - On user confirmation: fill form fields
  - Highlight filled fields (green border)
  - Never call `form.submit()` or click submit buttons
- [ ] **3.2** `src/fill.ts`:
  - Fill strategies per field type (text input, select, textarea, radio, checkbox)
  - Dispatch input/change events after fill (required by React-controlled forms)
  - Handle multi-field groups (work history, education)

### 4. Extension: Popup UI

- [ ] **4.1** `src/popup/` — HTML + CSS + TS:
  - Auth status indicator (connected / disconnected)
  - "Connect to Job Finder" button → opens dashboard for bootstrap code
  - Bootstrap code input field
  - Profile summary (name, skills count)
  - "Fill Form" button (manual trigger)
  - Link to dashboard
- [ ] **4.2** `src/popup/index.html`
- [ ] **4.3** `src/popup/popup.ts`

### 5. Extension: Manifest and build

- [ ] **5.1** Update `vite.config.ts` manifest:
  - Add popup (`action.default_popup`)
  - Add service worker background script
  - Add content script with proper matches
  - Scope host_permissions to known ATS domains + API origin
  - No `<all_urls>`
- [ ] **5.2** Verify build produces valid MV3 extension

### 6. Hard product line verification

- [ ] **6.1** Grep for `form.submit()`, `.submit()`, `click()` on submit buttons — must be zero
- [ ] **6.2** Confirm no `<all_urls>` in manifest
- [ ] **6.3** Confirm token scope is `profile:read` only
- [ ] **6.4** Confirm ambiguous fields are left blank (honest omission)

### 7. Verify

- [ ] **7.1** Extension builds: `pnpm --filter extension build`
- [ ] **7.2** Load unpacked extension in Chrome
- [ ] **7.3** Auth flow: dashboard → bootstrap code → extension connected
- [ ] **7.4** Profile fetch returns correct fields
- [ ] **7.5** Form fill on Greenhouse/Lever/Workday test pages
- [ ] **7.6** No auto-submit occurs

## Dependencies
- Existing `apps/api` for auth/profile endpoints
- Existing `apps/dashboard` for bootstrap code UI
- Existing extension detection core (ats.ts, detect.ts, maps.ts, heuristics.ts)
