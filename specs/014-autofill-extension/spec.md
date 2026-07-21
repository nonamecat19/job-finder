# 014 Application Form Autofill Extension — Spec

## Overview

A browser extension that reads the user's stored job-finder profile (skills, work history,
education) and fills matching fields on application forms. The extension populates fields;
the user clicks submit. No auto-submit, no background submission, ever.

This is a separate build and release artifact from the web app (dashboard / API). The
extension source lives at `apps/extension` in the repo.

---

## 1. Extension Architecture

### 1.1 Manifest V3

The extension uses Manifest V3 (MV3). Key structural decisions:

| Component | MV3 equivalent | Role |
|---|---|---|
| Background page | **Service worker** (non-persistent, event-driven) | Token refresh, API calls, message relay |
| Content script | **Content script** (isolated world per tab) | DOM inspection, field filling, status overlay |
| Popup | **Action popup** (optional, `action.default_popup`) | Status indicator, manual trigger, link to dashboard |
| Storage | `chrome.storage.session` | In-memory tokens per browser session |
| Storage | `chrome.storage.local` | Cached profile fields, user preferences |

No `webRequest` blocking — the extension does not intercept or modify network requests from
the host page. Host permissions are scoped narrowly per detected ATS domain and never
include `<all_urls>`.

### 1.2 Service Worker

The service worker is the extension's runtime backbone. It handles:

- **Token lifecycle**: Requests a short-lived access token from the API on install / on
  session start, stores it in `chrome.storage.session` (cleared when browser closes).
- **Profile fetch**: Retrieves the user's profile fields from the API and caches them in
  `chrome.storage.local`.
- **Message relay**: Content scripts send `{action: "fill-<field>", ...}` messages; the
  service worker responds with the matching profile value. The content script never holds
  the token.

### 1.3 Content Script

Injected per ATS domain on `document_idle`. Responsibilities:

1. **Field detection** — Scan the DOM for form inputs and identify likely profile-matching
   fields (label text, `name` attribute, `autocomplete` attribute, `aria-label`).
2. **Field mapping** — Map detected field metadata to profile fields using the heuristics
   defined in 014-4.
3. **Fill UI** — Show the user what will be filled (popup badge or inline overlay), wait for
   explicit confirmation, then populate the fields.
4. **Highlight filled fields** — Apply a visual indicator (e.g. green border) to filled
   fields so the user can review before submitting.

The content script communicates with the service worker via `chrome.runtime.sendMessage`.
It never has access to the auth token and never makes API calls directly.

### 1.4 Communication Flow

```
User visits ATS form page
  → Content script injected (manifest host_permissions match)
  → Content script scans DOM for form fields
  → Content script sends identify message to SW with detected fields
  → SW returns matching profile fields from cache
  → Content script renders "fields ready to fill" indicator
  → User clicks "Fill" button in popup/overlay
  → Content script fills form fields
  → Content script highlights filled fields (green border)
  → User reviews and clicks the ATS's own submit button
```

---

## 2. Authentication

### 2.1 No Long-Lived Secret

The extension MUST NOT embed or persistently store an API key or refresh token. The
authentication flow is:

1. **User authenticates in the dashboard web app** (existing login flow, session cookie /
   HTTP-only cookie).
2. **Dashboard generates a one-time bootstrap code** — a short-lived (5 minute) code
   displayed as a QR or string in the dashboard UI.
3. **User enters/approves the code in the extension** — the extension sends the code to
   `POST /api/v1/ext/auth/exchange`.
4. **API validates the code** and returns:
   - An **access token** (JWT, 1-hour TTL) — stored in `chrome.storage.session`
     (ephemeral, cleared on browser close)
   - A **refresh token** (opaque, 30-day TTL, one-time-use rotation) — stored in
     `chrome.storage.local` and rotated on each refresh

### 2.2 Token Scope and Security

| Property | Value |
|---|---|
| Token type | JWT (access), opaque (refresh) |
| Access token TTL | 1 hour |
| Refresh token TTL | 30 days (rotated on use) |
| Scope | `profile:read` only |
| Storage (access) | `chrome.storage.session` — never persisted to disk |
| Storage (refresh) | `chrome.storage.local` — encrypted at rest by Chrome |
| API endpoint | `POST /api/v1/ext/auth/exchange` (bootstrap code → token pair) |
| API endpoint | `POST /api/v1/ext/auth/refresh` (refresh token → new token pair) |

The access token is scoped to `profile:read` via a custom claim (`scope: "profile:read"`).
The API enforces this scope on every request from the extension. A compromised extension
cannot mutate account data, read other users' profiles, or access any dashboard API.

### 2.3 Token Refresh Lifecycle

```
SW starts / wakes up
  → Check session storage for access token
  → If missing or expired → read refresh token from local storage
  → POST /api/v1/ext/auth/refresh with refresh token
  → API returns new access token + rotated refresh token
  → Store access token in session storage
  → Store new refresh token in local storage (old one invalidated)
```

If the refresh token is also expired or invalidated, the user must re-authenticate via the
dashboard bootstrap code flow.

---

## 3. Profile Fields Exposed

The extension accesses the following profile fields via `GET /api/v1/ext/profile`. The
endpoint returns only the authenticated user's own profile.

### 3.1 Field Inventory

| Field | Type | Example | ATS target attributes |
|---|---|---|---|
| `fullName` | string | "Jane Doe" | `name="name"`, `autocomplete="name"` |
| `email` | string | "jane@example.com" | `name="email"`, `autocomplete="email"` |
| `phone` | string | "+1-555-1234" | `name="phone"`, `autocomplete="tel"` |
| `location` | string | "San Francisco, CA" | `name="location"`, `autocomplete="address-level1"` |
| `headline` | string | "Senior Frontend Engineer" | label contains "headline" or "title" |
| `skills` | string[] | `["React", "TypeScript", "Go"]` | `name="skills"`, custom ATS fields |
| `workHistory` | WorkEntry[] | (see below) | Multi-field employer/role/date groups |
| `education` | EducationEntry[] | (see below) | Multi-field institution/degree/date groups |
| `links` | Link[] | `[{url: "https://...", label: "Portfolio"}]` | `name="portfolio"`, `name="linkedin"` |

### 3.2 WorkEntry Shape

```json
{
  "employer": "Acme Corp",
  "role": "Senior Engineer",
  "startDate": "2020-03",
  "endDate": null,
  "current": true,
  "description": "Built React component library"
}
```

### 3.3 EducationEntry Shape

```json
{
  "institution": "MIT",
  "degree": "B.S. Computer Science",
  "startDate": "2012-09",
  "endDate": "2016-06"
}
```

### 3.4 Endpoint Authorization

`GET /api/v1/ext/profile` **must** reject requests for any profile other than the
authenticated user's. The access token's subject claim (`sub`) is matched against the
profile's owner ID server-side. A token issued for user A MUST NOT return user B's profile.

---

## 4. Field Detection and Mapping

(Detailed spec in 014-4 — this section summarizes the design boundary.)

The content script detects form fields using a priority-ordered strategy:

1. **`autocomplete` attribute** — Match against the standard autofill token list
   (e.g. `given-name`, `family-name`, `email`, `tel`).
2. **`name` attribute** — Match against known ATS field names (per-ATS maps for Greenhouse,
   Lever, Workday).
3. **Label text** — Fuzzy-match visible label text against field names.
4. **`aria-label` attribute** — Use as fallback when no label or name is available.

Ambiguous fields are left unfilled (honest omission preferred over wrong fill).

---

## 5. Project Layout

### 5.1 Repository Location

```
job-finder/
├── apps/
│   ├── api/                 # NestJS backend
│   ├── dashboard/           # Next.js dashboard
│   ├── extension/           # <-- NEW: Browser extension
│   └── jobspy-sidecar/      # Python scraper
├── packages/
│   └── shared/              # Shared types (extends with extension types)
├── specs/
│   └── 014-autofill-extension/
│       ├── spec.md          # This document
│       └── decision-log.md  # No-auto-submit decision log
└── pnpm-workspace.yaml      # Add apps/extension to packages list
```

### 5.2 Workspace Integration

`apps/extension` is added to the pnpm workspace via `pnpm-workspace.yaml` (the existing
glob `apps/*` already covers it). The extension has its own `package.json` with a `build`
script (Vite + `@crxjs/vite-plugin` or `@plasmohq/plasma` for MV3 builds) and a `lint`
script. It is NOT imported by `apps/dashboard` or `apps/api` at build time.

### 5.3 Separate Artifact

The extension builds to a `dist/` directory containing the unpacked MV3 bundle (manifest,
service worker, content scripts, popup HTML/CSS). This is loaded in Chrome via
`chrome://extensions → Load unpacked`. There is no bundling into the web app.

The extension is versioned independently from the rest of the monorepo (semver in
`apps/extension/package.json`).

---

## 6. Hard Product Lines

| Line | Rationale |
|---|---|
| **No auto-submit** | The user must review filled fields and click submit themselves. Auto-submit creates liability (wrong field filled, wrong value submitted), reduces trust, and the extension cannot observe post-submit outcomes reliably. |
| **No background submission** | The extension never calls `form.submit()` or clicks a submit button programmatically. Submission code paths must be absent from the bundle (test-enforced). |
| **No `<all_urls>` host permission** | Host permissions are scoped to known ATS domains + the API origin only. Additional domains require a manifest update. |
| **No API mutation** | The extension token is scoped `profile:read`. No CREATE, UPDATE, or DELETE operations are possible through the extension API surface. |
| **Honest omission > wrong fill** | Ambiguous fields remain blank. A wrong autofilled value the user does not notice is worse than an empty field they fill manually. |

---

## 7. Acceptance

- [ ] Spec document exists with category `spec`
- [ ] Spec covers extension architecture (manifest v3, content script vs service worker)
- [ ] Spec covers authentication scheme (no long-lived secret, short-lived tokens)
- [ ] Spec lists exposed profile fields and their ATS target attributes
- [ ] Spec defines extension source location (`apps/extension`) and artifact separation
- [ ] Spec documents all hard product lines
- [ ] Decision-log entry exists on the no-auto-submit line
- [ ] Spec registered in Documentation Directory
- [ ] Spec added to Documentation library
