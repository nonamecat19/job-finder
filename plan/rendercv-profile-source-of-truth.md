# Spec: RenderCV config as the single source of truth for the Profile

## Context

Today the Profile is a JSON Resume (`Profile.document` jsonb) seeded by a PDF import → LLM
draft; the Profile page (`apps/dashboard/src/features/profile/ProfilePage.tsx`) only lets a
user edit `name` + `extraNotes`. Separately, the RenderCV resume tailoring feature reads a
**global file** `resume/resume.yaml` (`RESUME_MASTER_PATH`) as its master — completely
disconnected from the DB Profile. So there are two competing resume representations and the
"real" CV (the RenderCV YAML with the user's Typst theme and true experience highlights) is
not what feeds matching.

**Goal:** make the RenderCV YAML config the *only* profile input. The user uploads one
`.yaml` file; nothing else. Matching, fit scoring, and resume generation all derive from it.
Vacancy fit is scored against the resume's **achievements** = `experience[].highlights` in the
config. Until a valid config exists, the app's job features are hard-gated off.

Decisions locked with the user:
- **Replace fully** — drop the JsonResume `document` + PDF-import flow. Rendercv YAML is the
  sole stored profile data; matching/generation derive from the parsed YAML.
- **Store in DB per profile** — new Profile column holds the config; replaces file-based
  `RESUME_MASTER_PATH` as the tailoring master.
- **Hard gate** — no valid config ⇒ matching/generation/scoring disabled; dashboard shows an
  upload prompt.
- **Validate on upload** — parse YAML *and* run a `rendercv` render smoke test (must produce a
  PDF) before accepting.

## Data model

Migration `apps/api/internal/db/migrations/00001_init.sql` (`Profile` table, lines 75-84).
Add a new migration `00002_profile_rendercv.sql` (do NOT edit the applied init migration):

- Add `"rendercvConfig" jsonb` (nullable — null = no config = ungated features).
- Add `"rendercvYaml" text` (raw uploaded YAML, kept verbatim so re-render is byte-exact).
- Keep `embedding` / `embedModel` / `extraNotes` / `name`.
- `document` jsonb is now legacy: make it nullable in the new migration. Keep the column for a
  release rather than dropping (avoids breaking sqlc/other reads mid-migration); stop writing it.

Regenerate sqlc after editing `apps/api/internal/db/queries/profile.sql`:
- Replace `document` in `CreateProfile`/`UpdateProfile` with `rendercvConfig` + `rendercvYaml`.
- Add `GetProfileConfig` / a `ProfileHasConfig` boolean query for the gate.

## Backend

### 1. RenderCV parsing + derived text (new)
New file `apps/api/internal/profile/rendercv.go`:
- `ParseRendercv(yamlText string) (generation.RendercvMaster, error)` — reuse the same
  `yaml.Unmarshal` + `normalizeYAMLMap` normalization used in
  `apps/api/internal/generation/service.go:91-95`.
- `RendercvToText(master) string` — the replacement for `ProfileToText`
  (`apps/api/internal/profile/service.go:187-225`). Pull, in order: `cv.name`/headline,
  the summary section, every skill group's details, and **each experience entry's
  `highlights` (the achievements)**. Reuse the section walkers already in
  `apps/api/internal/generation/rendercv.go` (`cvSections`, line 115, and the experience/skill
  accessors) so text extraction and tailoring agree on structure.
- This text is what gets embedded (`RefreshEmbedding`) and what the fit prompt scores against —
  so vacancy fit is now computed from resume achievements, per the request.

### 2. Profile service (`apps/api/internal/profile/service.go`)
- `Create`/`Update` take `rendercvYaml` instead of `document`; parse → store `rendercvConfig`
  (jsonb) + `rendercvYaml` (text); recompute embedding from `RendercvToText`.
- Delete/replace `ProfileToText` usage; `toDto` (line 237+) stops unmarshalling `document`.
- Remove the PDF-import path (`pdfimport.go`) and its handler wiring.

### 3. Upload + validation handler (`apps/api/internal/httpapi/profiles.go`)
- Replace `POST /profiles/import` (PDF) with `POST /profiles/config` — multipart `.yaml`
  upload (or raw text body). Steps: parse YAML → assert a `cv:` block + required fields →
  run a **render smoke test** via `RenderCvRenderer.Render` (`internal/generation/rendercv_renderer.go`)
  into a temp dir; reject with a 422 + message if the CLI fails to produce a PDF. On success,
  persist config + yaml and refresh the embedding.
- Add `GET /profiles/config/status` → `{ hasConfig: bool }` for the frontend gate.

### 4. Matching (`apps/api/internal/matching/service.go`)
- `MatchJob` currently builds `profileText` via `profile.ProfileToText(doc, ExtraNotes)`
  (line 74). Switch to `RendercvToText(parsedConfig)` (+ optional extraNotes).
- **Hard gate:** if the profile has no `rendercvConfig`, skip the job entirely (no embedding
  compare, no LLM call) and return a sentinel/skip so the worker records "no profile config"
  rather than a bogus score. Fit prompt (lines 81-93) unchanged except its input is now the
  achievements-derived text.

### 5. Generation (`apps/api/internal/generation/service.go`)
- Master source moves from `os.ReadFile(s.masterPath)` (line 87) to the profile's stored
  `rendercvConfig`. Add a `masterFor(ctx, profileID)` that loads the config from the DB; keep
  `masterPath` only as a dev fallback when no profile exists.
- Drop / deprecate the JsonResume HTML path (`tailorResume`, `writeCoverLetter` HTML side)
  that depended on `Profile.document`; the RenderCV path becomes the only resume renderer.
  Cover-letter generation still runs, but grounded in `RendercvToText`.
- Gate: generation endpoints 409/precondition-failed when no config.

### 6. Config / wiring
- `apps/api/cmd/server/main.go` + `internal/config/config.go`: `RESUME_MASTER_PATH` becomes a
  dev-only fallback; document that the master now lives in the DB.

## Frontend (`apps/dashboard`)

### Profile page — rewrite `src/features/profile/ProfilePage.tsx`
- Remove the JSON-resume display, name/notes edit cards, and PDF-import button.
- New single-purpose page: a `.yaml` file dropzone (reuse the hidden-input pattern at
  `ProfilePage.tsx:39`, change `accept=".pdf"` → `accept=".yaml,.yml"`) that POSTs to
  `/profiles/config`.
- States: **No config** → big upload prompt explaining "upload your RenderCV config to begin".
  **Uploading/validating** → spinner ("rendering a test PDF…"). **Invalid** → show backend
  validation error. **Valid** → show a summary parsed from the config (name, headline, skill
  groups, experience companies + highlight counts) + a "Replace config" button + a link/preview
  of the smoke-test PDF.
- API layer `src/lib/api.ts:74-81`: replace `profiles.import` with `profiles.uploadConfig(file)`
  (FormData, already supported at line 20) and `profiles.configStatus()`.
- Hooks `src/features/profile/hooks.ts`: replace `useImportProfile` with `useUploadConfig` +
  `useConfigStatus`; keep query-key invalidation.

### App-wide hard gate
- Add a `useConfigStatus()` check in the shell (`src/app/shell.tsx`). When `hasConfig` is
  false, job/match/generate routes render a "Set up your profile first" panel linking to
  `/profile`, instead of their normal content. Jobs/match views stay reachable but inert.

### Shared types (`packages/shared/src/index.ts`)
- Replace `ProfileDto.document: JsonResume` with `rendercvConfig` presence + a lightweight
  parsed summary type (`{ name, headline, skillGroups: string[], experience: {company, highlightCount}[] }`).
- Keep `JsonResume` types only if still referenced by legacy generated docs; otherwise remove.

## Delete / deprecate
- `apps/api/internal/profile/pdfimport.go` and its tests.
- PDF-import UI + `profiles.import` client method.
- Legacy `apps/dashboard/src/pages/ProfilePage.tsx` (already unused).
- `ProfileToText` (replaced by `RendercvToText`).

## Verification

1. `cd apps/api && go build ./... && go test ./internal/profile/... ./internal/matching/... ./internal/generation/...`
   — add a `rendercv_test.go` case for `RendercvToText` (asserts experience highlights land in
   the text) and a matching test asserting a config-less profile is skipped.
2. Run `sqlc generate` (config at `apps/api`) after editing queries; confirm no diff drift.
3. Boot the stack (docker-compose: postgres/pgvector, redis, ollama, rendercv CLI available).
   Apply the new migration.
4. Dashboard end-to-end (Chrome MCP or `pnpm --filter @job-finder/dashboard dev`):
   - Fresh DB → `/profile` shows the upload prompt; job routes show the gate panel.
   - Upload `resume/resume.yaml` → validation spinner → success summary; smoke-test PDF opens.
   - Upload a deliberately broken YAML → 422 + visible error, no profile saved.
   - After a valid upload, trigger a match on a seeded job → fit score returns and its
     `summary`/`matchedSkills` reflect achievements from the config's experience highlights.
5. Confirm `resume/resume.yaml` file is no longer read at runtime for a configured profile
   (grep server logs / temporarily rename the file and re-run a match+generate).
