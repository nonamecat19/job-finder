# 04 — React Dashboard

## Stack

- Vite + React 19 + TypeScript, `@job-finder/shared` for DTO types.
- TanStack Query for server state (polling for ingestion/generation status), React Router.
- Tailwind + shadcn/ui — fast, decent-looking, no design effort.
- Served by nginx container; proxies `/api` to the NestJS app (no CORS pain).

## Pages

### 1. Job Feed (`/`)
- Default view: jobs sorted by fit score desc.
- Filters: source, min score, remote, status, search text, date.
- Row: title, company, score badge (color-graded), matched/missing skill chips (top 3), source icon, posted date.
- Row actions: shortlist, hide, open original posting.

### 2. Job Detail (`/jobs/:id`)
- Full JD, fit breakdown (score, matched skills, missing skills, summary, red flags).
- Buttons: **Generate resume**, **Generate cover letter** (show spinner while queued/generating — local LLM takes tens of seconds), **Download PDF**, **Regenerate** (bumps version, history list below).
- Inline preview of generated cover letter text (editable before PDF render — user tweaks, then re-render).
- "Mark applied" → moves to tracker.

### 3. Profile (`/profile`)
- Master profile editor: structured sections (work, education, skills, projects) mapped to JSON Resume fields + free-form "extra notes" textarea.
- **Import resume**: PDF upload → parsed draft shown side-by-side for review/merge before save.
- Multiple profiles supported (dropdown), one active default.

### 4. Sources & Searches (`/sources`)
- Job source list: enable toggle, health indicator (green/red from `SourceRun` stats), credential/config form per source, "Test" button (runs `healthCheck`/tiny search).
- Saved searches CRUD: keywords, location, remote, source multi-select, cron preset dropdown (hourly/6h/daily), "Run now" button.
- Last-runs table: per run — source, found, new, error.

### 5. Tracker (`/tracker`)
- Kanban: columns = `shortlisted / docs_generated / applied / interview / offer / rejected`, drag-and-drop status change (dnd-kit).
- Card: company, title, days-in-column, note icon. Click → job detail.

## API contract (NestJS controllers, all under `/api`)

```
GET    /jobs?sort=score&source=&minScore=&status=&q=&page=
GET    /jobs/:id
POST   /jobs/:id/shortlist | /hide
POST   /jobs/:id/generate        { type: 'resume' | 'cover_letter' }   → 202 + poll
GET    /jobs/:id/documents        (versions, pdf links)
PUT    /documents/:id             (edit cover letter text before render)
GET    /documents/:id/pdf

GET/PUT/POST /profiles, POST /profiles/import (multipart PDF)

GET/PUT      /sources, POST /sources/:key/test
GET/POST/PUT/DELETE /searches, POST /searches/:id/run

GET    /applications?status=      PATCH /applications/:id { status, notes }
GET    /stats                     (feed counts, runs, pipeline summary)
```

Generation is async (BullMQ): `POST .../generate` returns a job id; the dashboard polls `GET /jobs/:id/documents` until the new version appears. WebSocket/SSE is a later nicety, polling is fine for one user.
