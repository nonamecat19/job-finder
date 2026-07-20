# Feature Specification: Recruiter / Hiring-Manager Resolution

**Feature Branch**: `007-recruiter-resolution`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "Parse posting + company page + (opt-in) LinkedIn company page to identify who owns the req. Output name, title, LinkedIn URL, email, phone, source, confidence — feeds outreach, but outreach itself is out of scope: read-only resolution, no auto-messaging."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See the resolved contact on the job detail page (Priority: P1)

A job seeker opens a job in their feed and, alongside the company intel card, sees a **Contact** line naming the most likely person who owns the requisition — a recruiter or hiring manager — with their title. When nothing could be resolved, the line reads "No contact found — try Refresh" rather than being absent, so the user can tell "we looked and found nobody" from "we never looked".

**Why this priority**: Knowing *who* to reach is the single highest-leverage piece of outreach preparation, and the posting-text parser (always-on, no opt-in, no external fetch) already resolves a contact for the subset of postings that name one. On its own this is a complete, shippable increment: one line on the detail page, powered entirely by data the system already has.

**Independent Test**: Ingest a job whose description body contains `Contact: Jane Doe, Recruiter <jane@acme.com>`, open its detail page, and confirm the Contact line shows "Jane Doe — Recruiter" with the email. Ingest a job whose body names no one and confirm the line shows the "No contact found" message. No company-page or LinkedIn fetch is needed for either.

**Acceptance Scenarios**:

1. **Given** a job whose description names a contact, **When** the detail page loads, **Then** the Contact line shows the highest-confidence resolved contact's name and title, and shows email/phone/LinkedIn URL when those were resolved.
2. **Given** a job for which no contact was resolved from any enabled source, **When** the detail page loads, **Then** the Contact line shows "No contact found — try Refresh" rather than being blank or absent.
3. **Given** two contacts resolved for one job, **When** the detail page loads, **Then** the Contact line surfaces the one with the highest confidence, breaking ties deterministically.
4. **Given** a posting whose only contact is a generic address such as `jobs@acme.com` with no person's name, **When** resolution runs, **Then** no named contact is fabricated; the generic address is either recorded as a low-confidence unnamed contact or dropped, and the detail line falls back to the "No contact found" state rather than presenting the mailbox as a person.

---

### User Story 2 - Refresh contacts on demand (Priority: P2)

The same job seeker, unsatisfied with what was resolved at ingest time (or looking at a job discovered before this feature existed), clicks **Refresh contacts** on the detail page. Resolution re-runs across all enabled sources — the posting text, the company team page (from plan 004's company row), and, *only when the operator has enabled it*, the LinkedIn company page — and the Contact line updates in place.

**Why this priority**: Discovery (Story 1) is usable on its own, but contacts change, company pages get parsed better over time, and the LinkedIn source is opt-in and won't have run at ingest for most users. An explicit re-run makes the richer sources reachable without re-ingesting the job. It ships independently of Story 3.

**Independent Test**: Open a job with no resolved contact, ensure its company has a `website` populated (plan 004), click Refresh contacts, and confirm the company team-page parser runs and any resolved contacts appear. With `LINKEDIN_SCRAPE_ENABLED=false`, confirm the LinkedIn source is silently skipped and the refresh still completes.

**Acceptance Scenarios**:

1. **Given** a job detail page, **When** the user clicks Refresh contacts, **Then** resolution re-runs across all enabled sources and the Contact line reflects the new best result when the run completes.
2. **Given** `LINKEDIN_SCRAPE_ENABLED` is false, **When** Refresh contacts runs, **Then** the LinkedIn source is silently skipped, no LinkedIn request is made, and the run completes using posting + company-page sources only.
3. **Given** `LINKEDIN_SCRAPE_ENABLED` is true, **When** Refresh contacts runs, **Then** the LinkedIn company-page People section is scraped and any resolved contacts are recorded with `source='linkedin'`.
4. **Given** a re-run resolves a contact already stored for that job from the same source with the same name, **When** results are persisted, **Then** the existing row is updated in place rather than duplicated (the `(jobId, source, name)` uniqueness).
5. **Given** the company has no `website` and LinkedIn is disabled, **When** Refresh contacts runs, **Then** only the posting-text source runs, the run completes without error, and the Contact line reflects whatever the posting yielded.

---

### User Story 3 - Expand the full candidate list (Priority: P3)

A job seeker who wants more than the single best contact expands the Contact line into a list of every candidate resolved for that job. Each entry shows the person's name, title, whatever contact channels were found (email, phone, LinkedIn URL), the **source** it came from (posting / company-page / linkedin), and a **confidence** indicator — so the user can judge how much to trust each one before reaching out themselves.

**Why this priority**: The single best contact (Story 1) covers the common case. Power users chasing a specific role want to see the recruiter *and* the hiring manager *and* the team lead, and to weigh a high-confidence posting match against a lower-confidence LinkedIn guess. Additive; breaks nothing in Stories 1-2.

**Independent Test**: For a job with contacts resolved from more than one source, expand the Contact line and confirm each candidate is listed with its source label and confidence, ordered best-first.

**Acceptance Scenarios**:

1. **Given** a job with multiple resolved contacts, **When** the user expands the Contact line, **Then** every stored contact for that job is listed, each showing name, title, available channels, source, and confidence.
2. **Given** the expanded list, **When** it renders, **Then** contacts are ordered by confidence descending, with the same deterministic tie-break used to pick the headline contact.
3. **Given** a job with exactly one resolved contact, **When** the user expands, **Then** the single contact is shown and the list does not misrepresent it as a partial set.

---

### Edge Cases

- **No contact found anywhere** — posting names no one, company page has no team/about section (or the company has no `website`), and LinkedIn is disabled or yields nothing. Resolution completes successfully with zero contacts; the detail line shows the "No contact found — try Refresh" state.
- **LinkedIn disabled** — `LINKEDIN_SCRAPE_ENABLED` is false (the default). The LinkedIn source is silently skipped; no request is made; the run is not marked failed for skipping it.
- **Generic mailbox only** — the posting contains `jobs@`, `careers@`, `hr@` and no personal name. No person is invented; the address is recorded as an unnamed low-confidence contact or dropped, never surfaced as a named human.
- **Company page has no People/Team section** — the parser finds no candidates; the company-page source contributes zero contacts without erroring.
- **Multiple recruiters** — several plausible contacts resolve for one job; the highest-confidence one is surfaced on the headline line and the rest are available in the expanded list.
- **Same person from two sources** — e.g. resolved from both the posting and LinkedIn. Because uniqueness is `(jobId, source, name)`, the person is stored once per source; the UI may show both rows (different provenance) and the higher-confidence one wins the headline.
- **LinkedIn markup / gating change** — the LinkedIn People section fails to parse or is blocked. The LinkedIn source degrades to zero contacts with a warning; posting + company-page results are unaffected.
- **Stale contact after job removal** — when a `Job` is deleted, its `JobContact` rows are removed by FK cascade; no orphaned contacts remain.
- **Malformed contact data in the posting** — an email or phone that does not validate is not stored as a channel; the contact may still be recorded from its name if a name was present.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST resolve likely recruiter/hiring-manager contacts for a job from up to three sources — the posting text, the company team/about page, and (opt-in) the LinkedIn company page — and persist each as a `JobContact` row.
- **FR-002**: System MUST always run the posting-text source; it requires no opt-in, no external fetch, and produces contacts with `source='posting'`.
- **FR-003**: System MUST run the company team-page source using the company `website` populated by plan 004 (Company Intel Card), reusing plan 004's company-page fetch, and produce contacts with `source='company-page'`.
- **FR-004**: System MUST gate the LinkedIn company-page source behind the `LINKEDIN_SCRAPE_ENABLED` environment variable, which defaults to **false**; when false the source is silently skipped and no LinkedIn request is made.
- **FR-005**: System MUST, when `LINKEDIN_SCRAPE_ENABLED` is true, scrape the public LinkedIn company-page People section for recruiters and hiring managers and produce contacts with `source='linkedin'`.
- **FR-006**: System MUST record, for each resolved contact, the fields it could determine: name, title, LinkedIn URL, email, phone, the source it came from, and a confidence score; unknown channels are stored as absent, not fabricated.
- **FR-007**: System MUST NOT fabricate a person: a generic mailbox (e.g. `jobs@`, `hr@`) with no accompanying human name MUST NOT be surfaced as a named contact. It MAY be stored as an unnamed low-confidence contact or dropped.
- **FR-008**: System MUST ground every resolved field in observed source text — the posting body, the company page, or the LinkedIn page — never inferring a person, title, or channel not present in the source (Constitution Principle II).
- **FR-009**: System MUST surface, on the job detail page, the single highest-confidence resolved contact for the job, or an explicit "No contact found — try Refresh" state when there are none.
- **FR-010**: System MUST break confidence ties deterministically so the headline contact and the list ordering are stable across renders for unchanged data.
- **FR-011**: System MUST provide a "Refresh contacts" action that re-runs resolution across all enabled sources for a single job and updates the stored contacts.
- **FR-012**: System MUST expose the full set of resolved contacts for a job as an expandable list, each entry showing source and confidence.
- **FR-013**: System MUST enforce `UNIQUE(jobId, source, name)` on `JobContact` so a re-run updates an existing contact in place rather than duplicating it.
- **FR-014**: System MUST cascade-delete a job's `JobContact` rows when the `Job` is deleted (`ON DELETE CASCADE`).
- **FR-015**: System MUST confine a failure of any one source (LinkedIn blocked, company page unreachable, posting unparseable) to that source, still persisting contacts from the sources that succeeded and not marking the whole resolution failed.
- **FR-016**: System MUST complete a resolution that yields zero contacts without erroring, recording zero rows and leaving the detail line in the "No contact found" state.
- **FR-017**: System MUST NOT send any message, email, connection request, or application to any resolved contact; resolution is strictly read-only (Constitution Principle I). Outreach is a separate, out-of-scope feature.
- **FR-018**: System MUST treat contact channels (email, phone, LinkedIn URL) as sensitive personal data — never written to logs in full and never exposed beyond the job detail surface that already shows them.
- **FR-019**: System MUST make LinkedIn scraping togglable without a redeploy of the source code — the `LINKEDIN_SCRAPE_ENABLED` env var is read at process start, matching the existing config surface.

### Key Entities

- **JobContact**: A resolved candidate owner of a requisition for one job. Holds the person's name, optional title, optional LinkedIn URL / email / phone, the `source` that produced it (`posting` / `company-page` / `linkedin`), a `confidence` float, and a `fetchedAt` timestamp. Belongs to exactly one `Job` (FK, cascade delete). Unique per `(jobId, source, name)`.
- **Job** (existing): The requisition contacts are resolved for. Provides `Job.id` (FK target) and its `description` body (the posting-text source input).
- **Company** (existing, from plan 004): Provides the `website` the company team-page source fetches. The company-page fetch itself is reused from plan 004's `internal/companyintel` package.
- **Resolution Source**: One of the three producers of contacts — posting-text (always on), company-page (needs `Company.website`), linkedin (opt-in via env var). Each stamps its `source` value on the rows it produces.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a posting whose body explicitly names a contact (e.g. `Contact: Jane Doe, Recruiter <jane@acme.com>`), the resolved headline contact carries that name and title, and the email when present — with no manual step beyond opening the job.
- **SC-002**: For a posting that names no one, the detail line shows the "No contact found — try Refresh" state, never a blank line and never a fabricated name.
- **SC-003**: A generic mailbox (`jobs@`, `hr@`, `careers@`) with no personal name is never displayed as a named person — 100% of such cases resolve to the "No contact found" headline or an explicitly unnamed low-confidence row.
- **SC-004**: With `LINKEDIN_SCRAPE_ENABLED=false`, a full resolution makes zero requests to LinkedIn — verifiable from request logs — and still completes.
- **SC-005**: Clicking Refresh contacts re-runs resolution and the detail line reflects the new best result within the time it takes the enabled sources to fetch, without a page reload.
- **SC-006**: Re-running resolution twice for the same job with unchanged source data adds zero duplicate `JobContact` rows.
- **SC-007**: When one source fails (e.g. LinkedIn blocked), 100% of contacts from the other sources are still persisted and shown.
- **SC-008**: Every contact shown in the expanded list is attributable to its source and confidence without reading logs.
- **SC-009**: Deleting a job removes 100% of its `JobContact` rows, leaving no orphans.
- **SC-010**: For a job with contacts from more than one source, the headline contact is the highest-confidence one, and the ordering is identical across repeated page loads of unchanged data.

## Assumptions

- **Plan 004 has landed first**: `Company.website` exists and is populated, and the `internal/companyintel` package's company-page fetch is available for reuse. This feature does not build its own company-page fetcher (see Dependencies).
- **Read-only resolution**: Per the project's non-negotiable no-auto-apply principle, this feature only *identifies* contacts. Reaching out to them is a separate, later feature and is out of scope here.
- **Posting-text is the always-on baseline**: The posting-text source needs no external fetch and no opt-in; it is the minimum viable source and the P1 story rests on it alone.
- **LinkedIn is opt-in and off by default**: The LinkedIn source is a ToS gray area (see plan.md Constitution Check); it stays disabled unless the operator sets `LINKEDIN_SCRAPE_ENABLED=true`, and the default keeps the system fully functional and inside clear lines without it.
- **LLM extraction is grounded and local**: Contact extraction from free text uses the local Ollama runtime (Constitution Principle V), and every extracted field must be traceable to source text (Principle II) — no invented people.
- **Confidence is a producer-assigned float in [0,1]**: Each source assigns confidence by how directly the source names the person as owning the req (an explicit "Contact:" line ranks above a name scraped from a team page). Exact scoring is an implementation concern for the resolution use-case task, not this spec.
- **One person can legitimately appear under multiple sources**: The `(jobId, source, name)` uniqueness intentionally allows the same name from `posting` and `linkedin` as two rows with distinct provenance.
- **Company pages and LinkedIn may change layout at any time**: Parsing is best-effort and defensive; a layout change degrades that source to zero contacts with a warning, not an error (mirrors spec 001's stance).

## Dependencies

- **Plan 004 (Company Intel Card)** — REQUIRED, must land before implementation. Provides `Company.website` and the reusable company-page fetch in `internal/companyintel`. Implementation tasks for this feature MUST NOT start until 004-4 (the `internal/companyintel` package) has landed.
- The existing `Job` table and job detail page (the surface the Contact line attaches to).
- The existing local LLM runtime (Ollama) used for grounded free-text extraction.
- The existing scraping/page-fetch service (shared pacing, headers, error handling) for the company-page and LinkedIn fetches.
- The existing config surface for reading `LINKEDIN_SCRAPE_ENABLED` at process start.
- LinkedIn's public company page remaining reachable when the opt-in is enabled — an external dependency outside the project's control, and a ToS gray area addressed in plan.md.
