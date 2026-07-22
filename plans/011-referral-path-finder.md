# Plan: 011 Referral Path Finder

**Status**: Not started. Zero code exists — no migration, no Go package, no dashboard.

**Note**: This feature has no spec document in `specs/`. The Stoneforge tasks reference:
- `011-1` Spec for referral path finder (closed)
- `011-2` Migration 00013_contact_graph.sql (in_progress)
- `011-3` Contacts CSV import (open)
- `011-4` GitHub cross-reference (open)
- `011-5` Warm-path ranking + UI (open)

## What Exists

| Layer | Status |
|-------|--------|
| Spec document | **Missing** — no `specs/011-*/` directory |
| SQL migration | **Missing** |
| Go package | **Missing** |
| Dashboard | **Missing** |

## Tasks

### 1. Create spec (if missing)

- [ ] **1.1** Check if spec exists elsewhere or was deleted
- [ ] **1.2** If missing, create minimal spec based on Stoneforge task descriptions:
  - Contact graph: store user's professional contacts (CSV import)
  - GitHub cross-reference: find mutual connections via GitHub
  - Warm-path ranking: score referral paths by connection strength
  - UI: show referral paths on job detail or outreach panel

### 2. Schema

- [ ] **2.1** Create migration `00013_contact_graph.sql`:
  - `Contact` table: name, email, company, role, LinkedIn URL, GitHub username, source (csv/github)
  - `ContactConnection` table: from_contact, to_contact, relationship type, strength
  - Indexes for path-finding queries
- [ ] **2.2** Apply migration, verify goose up/down

### 3. Backend

- [ ] **3.1** `apps/api/internal/referral/` package:
  - CSV import parser
  - GitHub cross-reference (public API, no auth needed for public data)
  - Graph path-finding (BFS/DFS from user to target company contacts)
  - Warm-path ranking (connection strength scoring)
- [ ] **3.2** sqlc queries for contact CRUD and path queries
- [ ] **3.3** HTTP endpoints:
  - `POST /api/contacts/import` — CSV upload
  - `GET /api/jobs/{id}/referral-paths` — Find paths to company
  - `POST /api/contacts/github-sync` — Sync GitHub connections

### 4. Dashboard

- [ ] **4.1** CSV import UI (upload + preview)
- [ ] **4.2** Referral path display on job detail or outreach panel
- [ ] **4.3** Warm-path ranking visualization

### 5. Verify

- [ ] **5.1** `go test ./internal/referral/...` passes
- [ ] **5.2** `make test-lint` passes

## Dependencies
- None from other plans (can be built independently)
- GitHub public API (no auth needed for public profile data)
