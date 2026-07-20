# Implementation Plan: Recruiter / Hiring-Manager Resolution

**Branch**: `007-recruiter-resolution` | **Date**: 2026-07-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/007-recruiter-resolution/spec.md`

## Summary

Resolve who owns a job's requisition from up to three read-only sources — the posting text (always on), the company team/about page (reusing plan 004's company-page fetch), and the LinkedIn company page (opt-in, off by default) — persist each candidate as a `JobContact` row, and surface the best one on the job detail page with an expandable full list. Output per contact: name, title, LinkedIn URL, email, phone, source, confidence. **Outreach is out of scope**: this feature identifies people, it never contacts them.

**Dependency gate**: implementation MUST NOT start until plan 004's `internal/companyintel` package (task 004-4) has landed — the company-page source reuses its fetch. The spec bundle (this task) has no such gate and is written now.

## Technical Context

**Language/Version**: Go 1.23+ (`apps/api`); React + Vite dashboard for the detail-page UI.

**Primary Dependencies**: existing local Ollama runtime (grounded free-text extraction), existing scraping/page-fetch service, plan 004's `internal/companyintel` company-page fetch, `github.com/PuerkitoBio/goquery` (HTML parsing, already in the tree). No new third-party dependency.

**Storage**: PostgreSQL via sqlc — **one new table `JobContact`** and **one new goose migration** (`00010_job_contact.sql`; take the next free sequential version if taken). No change to existing tables.

**Testing**: `go test ./...` for the API (unit + Docker-backed integration for the cascade/upsert); `vitest` for the dashboard; live smoke behind a `live` build tag for the company-page and (opt-in) LinkedIn parsers. `make test-lint` is the boundary gate since the change spans `apps/api` and `apps/dashboard`.

**Target Platform**: Linux server (Docker Compose), same containers as the rest of the stack.

**Project Type**: Web service (Go API) + React dashboard.

**Performance Goals**: Not throughput-bound. Resolution is per-job, on ingest and on explicit Refresh; latency is dominated by external fetches, which are paced by the shared scraping service.

**Constraints**: LinkedIn scraping is gated by `LINKEDIN_SCRAPE_ENABLED` (default **false**) and is a ToS gray area (see Constitution Check). All three sources are strictly read-only (GET/parse only). Contact channels are sensitive PII (spec FR-018).

**Scale/Scope**: One migration, one new package/use-case for resolution with three source parsers, sqlc queries for `JobContact`, one detail-page UI addition. Single-user self-hosted deployment.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ PASS | Resolution is read-only: it parses posting text, fetches and parses public pages, and writes `JobContact` rows. No code path messages, emails, connects-with, or applies to any resolved contact. Outreach is explicitly out of scope (spec FR-017). This is the cleanest possible pass — the feature *prepares* a human action and never takes one. |
| **II. Grounded Generation** | ⚠️ **CONDITIONAL / gray-area — see below** | Two distinct risks live here: LLM contact-extraction can fabricate a person, and the LinkedIn source raises a terms-of-use question. Both are addressed below; neither is a clean tick. |
| **III. Typed Contracts** | ✅ PASS | `JobContact` reaches Go via sqlc regeneration, not a hand-written struct. Any field exposed to the dashboard flows through the existing tygo → `packages/shared` path; no hand-duplicated TS type. New migration follows the sequential-goose-version rule. |
| **IV. Test Discipline** | ✅ PASS | `go test` (unit over fixtures + Docker-backed integration for FK cascade and `(jobId, source, name)` upsert), `vitest` for the UI, live smoke behind the `live` tag for the page parsers. `make test-lint` gates the merge because the change crosses `apps/api` and `apps/dashboard`. |
| **V. Local-First, Self-Hosted** | ✅ PASS (opt-in default off) | Contact extraction runs on the local Ollama runtime — no third-party paid AI API in the resolution path. External fetches (company page, LinkedIn) are *discovery*, not inference, which the principle permits. The one external, ToS-sensitive source (LinkedIn) is opt-in and **defaults to false**, so the system is fully functional, and inside clearly-permitted lines, out of the box. Enabling it is a deliberate operator choice. |

### Principle II — the honest version

**Risk 1 — fabricated people (the hallucination risk).** The posting-text and page parsers use the local LLM to pull a name, title, and channels out of free text. Principle II binds LLM output to source data and forbids fabrication. A resolver that invents "Jane Doe, Senior Recruiter" for a posting that named no one is exactly the harm the principle exists to prevent, and here it is worse than a bad resume line: a user could email a person who does not exist, or worse, the wrong real person.

The design's answer, and the condition under which this passes:

1. **Every field must trace to observed source text** (spec FR-008). The extractor is prompted and post-processed to return only spans it saw; a name absent from the input is never emitted. A unit test asserts this against a fixture that contains channels but no name.
2. **No person is invented from a mailbox** (spec FR-007, SC-003). `jobs@`, `hr@`, `careers@` with no human name never becomes a named row — it is dropped or stored explicitly unnamed at low confidence. This is the single most likely fabrication path and it is closed at the data layer (`name NOT NULL`, and the resolver refuses a sentinel).
3. **Confidence + source are always carried** (spec FR-006, FR-012, SC-008), so a low-confidence LinkedIn guess is never presented with the same weight as an explicit `Contact:` line in the posting. The user judges before acting.

Under those three, the LLM source is grounded and Principle II passes. They are non-negotiable, not polish — remove them and the feature violates the principle.

**Risk 2 — the LinkedIn terms-of-use question (the gray area the task flags).** This is not really about hallucination; it is about *how the LinkedIn data is obtained*, and it does not resolve to a clean PASS.

- **LinkedIn's User Agreement prohibits automated scraping** of the site. Scraping the public company-page People section is therefore against LinkedIn's stated terms, regardless of the page being publicly viewable. This is the *same category* as the robota.ua decision in [spec 001](../001-workua-robotaua-adapters/research.md) and the levels.fyi decision in [spec 006](../006-salary-inference/plan.md#principle-v), where this project chose to park or substitute a source rather than work around an operator's stated position.
- **Why this feature does not simply park it (the difference from robota.ua)**: robota.ua was parked because there was *no* usable path without circumventing Cloudflare, and the user got zero value. Here the feature ships and delivers real value on the posting-text and company-page sources alone; LinkedIn is a *third, optional* source. So the resolution taken is not "park it" but **"gate it, off by default, and make enabling it an explicit, informed operator choice"** — `LINKEDIN_SCRAPE_ENABLED=false` by default (spec FR-004, FR-019).
- **Rationale for allowing the opt-in at all**: this is a single-user, self-hosted tool (Principle V). The operator is the user, acting on their own machine, at human-browsing pace, reading a public page to prepare their own job search — not a company harvesting data at scale. The gate puts the ToS decision where it belongs: with the operator who bears it, not silently baked into the default build. The default-off posture means the shipped product makes no scraping request the operator did not knowingly turn on.
- **What is NOT acceptable**, and is out of scope: logging in, defeating auth walls or rate-limit challenges, or scraping non-public People data. The source reads only the public company page and degrades to zero contacts on any gating/markup change (spec edge case).

**Gate status**: Principle II passes **conditionally** — on the three grounding controls for Risk 1, and on the default-off, explicit-opt-in, public-only, read-only posture for Risk 2. `linkedin` as a source is a **knowingly-enabled operator option, not a default capability.** This is recorded in Complexity Tracking below as a deliberate, justified deviation from a clean pass rather than a silent one.

### Post-Phase-1 re-check

To be completed after data-model/contracts settle. Expected: still passing under the same conditions — the design adds no outreach path, no hand-written cross-language type, and no third-party inference API; the LinkedIn gate remains default-off.

## Complexity Tracking

| Item | Why it is not a clean pass | Why it is justified | Control |
|---|---|---|---|
| LinkedIn company-page scraping (`source='linkedin'`) | Against LinkedIn's User Agreement (automated scraping), a ToS gray area — same category as parked robota.ua / substituted levels.fyi. | Optional third source; feature ships fully without it. Single-user self-hosted operator reads a public page at human pace to prepare their own search. Decision placed with the operator who bears it. | `LINKEDIN_SCRAPE_ENABLED` default **false** (FR-004); public-page + read-only only; no auth defeat; silent skip when off (SC-004); degrades to zero on block. |
| LLM contact extraction | LLM producing a person from free text sits against Principle II's anti-fabrication rule. | It is the feature; grounded correctly it is exactly a "trace to source" transform, not invention. | Field-traceability (FR-008), no-mailbox-to-person (FR-007), confidence+source always carried (FR-006). |

## Project Structure

### Documentation (this feature)

```text
specs/007-recruiter-resolution/
├── plan.md              # This file
├── spec.md              # Feature spec (FR-NNN / SC-NNN)
├── research.md          # Phase 0: source-parsing findings + LinkedIn ToS decision record
├── data-model.md        # Phase 1: JobContact table + reused types
├── quickstart.md        # Phase 1: Levels 1-4 validation guide
└── tasks.md             # Phase 2 output (/speckit-tasks) — see companion tasks below
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── db/
│   │   ├── migrations/00010_job_contact.sql   # NEW — JobContact table (task 007, "Add JobContact table")
│   │   ├── queries/job_contact.sql            # NEW — upsert / list-by-job sqlc queries
│   │   └── integration_test.go                # MODIFIED — truncateAll: JobContact before Job
│   ├── companyintel/                          # REUSED from plan 004 — company-page fetch (do not fork)
│   ├── recruiter/                             # NEW — resolution use-case + three source parsers
│   │   ├── resolve.go                         #   orchestrates sources, upserts JobContact
│   │   ├── posting.go                         #   source='posting'  (LLM over Job.description)
│   │   ├── companypage.go                     #   source='company-page' (reuses companyintel fetch)
│   │   └── linkedin.go                        #   source='linkedin' (gated by env var)
│   ├── config/config.go                       # MODIFIED — LinkedInScrapeEnabled from LINKEDIN_SCRAPE_ENABLED
│   └── httpapi/                               # MODIFIED — GET contacts + POST refresh endpoints
└── ...
apps/dashboard/
└── src/                                       # MODIFIED — Contact line + expandable list on job detail
```

**Structure Decision**: Resolution is its own `internal/recruiter` package (parallel to `internal/companyintel`, `internal/matching`) so the three source parsers and the upsert orchestration live together and each parser is independently unit-testable over fixtures. The company-page fetch is *imported* from `internal/companyintel`, not reimplemented (the plan-004 dependency).

## Key Design Decisions

Full reasoning belongs in [research.md](./research.md). Summary:

1. **Three sources, one upsert path**: each parser returns `[]ResolvedContact`; the use-case upserts all of them on `(jobId, source, name)`. A source failure is caught per-source (FR-015) — the others still commit.
2. **Posting-text is always on and needs no fetch**: it runs over `Job.description` already in the DB, which is why the P1 story ships without any external call.
3. **Company-page reuses plan 004's fetch**: no second fetcher; `Company.website` is the input. Absent website ⇒ source skipped, not failed.
4. **LinkedIn is env-gated and default-off**: `LINKEDIN_SCRAPE_ENABLED` read at process start (FR-019). False ⇒ the parser is never invoked and no request is made (SC-004).
5. **Confidence-ranked headline, deterministic tie-break**: highest confidence wins the detail line (FR-009); ties break on a stable secondary key (e.g. source priority then name) so ordering is stable across renders (FR-010, SC-010).
6. **Channels are sensitive**: email/phone/LinkedIn URL are never logged in full (FR-018).

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| LLM fabricates a contact not in the source | Medium | Field-traceability tests (FR-008); no-mailbox-to-person rule (FR-007); confidence surfaced so users don't over-trust (FR-006). |
| LinkedIn blocks or changes markup | High (expected) | Source is opt-in; degrades to zero contacts with a warning (edge case); posting + company-page unaffected (FR-015). |
| Implementation starts before plan 004 lands | — | Hard dependency gate: no implementation task begins until 004-4 (`internal/companyintel`) has landed. |
| Duplicate contacts on re-run | Low | `UNIQUE(jobId, source, name)` + upsert (FR-013); integration test asserts stable row count (SC-006). |
| Orphaned contacts after job delete | Low | FK `ON DELETE CASCADE` + `truncateAll` order fix; cascade test (FR-014, SC-009). |
| ToS exposure from LinkedIn scraping | Operator-borne | Default-off, public-page-only, read-only, no auth defeat; decision placed with the operator (Constitution Check Risk 2). |

## Phase Status

- [x] Phase 0: research.md — source-parsing approach + LinkedIn ToS decision record
- [x] Phase 1: data-model.md, quickstart.md
- [ ] Constitution re-check post-design
- [ ] Phase 2: tasks.md — companion implementation tasks (gated on plan 004)
