# Phase 0 Research: Recruiter / Hiring-Manager Resolution

Decisions and their rationale for the three resolution sources, plus the LinkedIn terms-of-use decision record. Field mappings live in [data-model.md](./data-model.md); validation steps in [quickstart.md](./quickstart.md).

## Decision 1 — Three sources, posting-text always on

**Decision**: Resolve contacts from three independent sources — `posting`, `company-page`, `linkedin` — with the posting-text source always on and requiring no external fetch.

**Rationale**: The posting body is already in the DB (`Job.description`) and a meaningful fraction of postings name a contact inline (`Contact: Jane Doe, Recruiter <jane@acme.com>`). Extracting from it costs one local-LLM call and no network I/O, which is why User Story 1 (P1) ships on it alone. The other two sources add coverage but each carries a precondition (a company website / an operator opt-in), so they are additive layers, not the foundation.

**Alternatives rejected**:
- *Single combined scraper*: collapses provenance. The spec requires per-contact `source` (FR-006, SC-008), and the ToS posture differs per source — they cannot share one code path.
- *Company-page first*: most postings *do* have a company website but many company sites have no People/Team page, and the fetch+parse is far costlier than reading text already in hand. Posting-text is the cheaper, higher-precision baseline.

## Decision 2 — Posting-text extraction via the local LLM, grounded

**Decision**: Extract name/title/email/phone from `Job.description` with the local Ollama model, post-processed so every emitted field is a span present in the input.

**Rationale**: Contact lines are unstructured and multilingual; a regex-only approach misses "reach out to our talent partner Jane (jane at acme dot com)". The LLM handles the variety, but Constitution Principle II forbids fabrication — so the output is constrained to observed spans and validated after generation. Emails/phones are additionally regex-validated before being stored as channels (a malformed match is dropped, per spec edge case).

**The no-fabrication controls** (also in [plan.md](./plan.md) Constitution Check):
1. Field-traceability: a field absent from the input is never emitted.
2. No mailbox-to-person: `jobs@`/`hr@`/`careers@` with no human name never becomes a named row (FR-007, SC-003).
3. Confidence + source always carried, so downstream trust is calibrated.

**Alternatives rejected**:
- *Regex-only*: brittle against phrasing and Cyrillic/mixed text; misses obfuscated addresses.
- *A hosted extraction API*: violates Principle V (local-first, no third-party paid AI in the core path).

## Decision 3 — Company-page source reuses plan 004's fetch

**Decision**: The `company-page` source fetches `Company.website` using plan 004's `internal/companyintel` fetch, then LLM-parses the About/Team/People markup for recruiters and hiring managers. It does not implement its own fetcher.

**Rationale**: Plan 004 already fetches and caches company pages (funding, layoffs, headcount, tech-stack signals). Adding a second fetcher would duplicate pacing, headers, and error handling and risk double-fetching the same page. Reuse is the whole reason for the plan-004 dependency gate: implementation cannot start until 004-4 lands.

**Precondition handling**: no `Company.website` ⇒ the source is *skipped*, not failed (spec Story 2 scenario 5). No People/Team section ⇒ zero contacts, no error (edge case).

**Confidence**: lower than an explicit posting `Contact:` line — a name on a team page is *plausibly* the req owner, not *stated* to be. This is why confidence + provenance drive the headline pick (FR-009/FR-010).

## Decision 4 — LinkedIn source: env-gated, default-off, public-only

**Decision**: The `linkedin` source scrapes only the **public** LinkedIn company-page People section, and only when `LINKEDIN_SCRAPE_ENABLED=true` (default false). No login, no auth-wall defeat, no rate-limit-challenge circumvention, no non-public data.

**Rationale**: See the decision record below. In short: it is a real ToS gray area, so it is off by default and enabling it is an explicit operator choice.

**Skip semantics**: env false ⇒ the parser is never constructed/invoked; zero LinkedIn requests are made (SC-004). This is a *silent skip*, not a failure — the run is not marked failed for a disabled source (FR-004, FR-015).

## Decision 5 — Storage: one child table, upsert on (jobId, source, name)

**Decision**: Persist to a new `JobContact` table keyed uniquely on `(jobId, source, name)`, FK to `Job` with `ON DELETE CASCADE`. See [data-model.md](./data-model.md).

**Rationale**: A job has many contacts; contacts are meaningless without their job; a re-run must update in place, not duplicate. `(jobId, source, name)` uniqueness gives idempotent re-runs (FR-013, SC-006) while deliberately allowing the same person under two different sources (distinct provenance). `ON DELETE CASCADE` guarantees no orphans (FR-014, SC-009).

## Decision 6 — Headline pick and ordering

**Decision**: The detail-page headline contact is the highest-`confidence` row; ties break on a stable secondary key (source priority `posting` > `company-page` > `linkedin`, then name). The expanded list uses the same order.

**Rationale**: Confidence is the honest ranking signal; the deterministic tie-break makes the headline and list stable across renders of unchanged data (FR-010, SC-010) — important because a flickering "who's the contact" is worse than a slightly-arbitrary-but-stable one.

---

## Decision Record — LinkedIn scraping terms of use

**Question posed by the task**: LinkedIn company-page scraping is a "ToS gray area — document in plan.md Constitution Check." This record is the evidence and the reasoning behind the default-off opt-in.

**Findings**:
1. **LinkedIn's User Agreement prohibits automated collection.** Scraping any part of the site by automated means is against LinkedIn's stated terms, whether or not the page is publicly viewable without login. Public-viewability is not the same as scraping-permitted.
2. **This is the same category as two prior project decisions.** [Spec 001](../001-workua-robotaua-adapters/research.md) parked robota.ua rather than defeat its Cloudflare bot challenge; [spec 006](../006-salary-inference/plan.md#principle-v) recommended substituting an explicitly-licensed dataset rather than scrape levels.fyi against its terms. This project's precedent is to *not* silently work around an operator's stated position.

**Why this feature gates rather than parks** (the material difference from robota.ua):
- robota.ua left the user with **zero value** without circumvention — it was the whole source.
- Here, LinkedIn is the **third of three** sources. The feature ships and delivers real value on posting-text + company-page alone. So the proportionate response is not to park the whole feature but to make the ToS-sensitive source an **explicit, informed, default-off operator choice**.

**The posture chosen**:
- `LINKEDIN_SCRAPE_ENABLED` defaults to **false**. The shipped product makes no LinkedIn request the operator did not knowingly enable (SC-004).
- When enabled: **public company page only**, **read-only**, **human-browsing pace** via the shared scraping service. No login, no auth defeat, no non-public People data.
- Enabling it is a single-user self-hosted operator acting on their own machine to prepare their own job search — the actor who bears the ToS decision is the one making it, not a default baked into the build.
- Degrades to zero contacts with a warning on any block or markup change; never blocks the other sources.

**Explicitly out of bounds**: authenticated scraping, defeating rate-limit or bot challenges, scraping profiles behind a login wall, or any scale beyond one operator's own manual-pace research.

**Status**: recorded in [plan.md](./plan.md) Constitution Check (Principle II conditional / gray-area) and Complexity Tracking as a deliberate, justified, operator-borne deviation — not a silent one.

---

## Open items for Phase 2 (implementation, gated on plan 004)

- Exact confidence scoring per source (spec leaves it to the resolution use-case task).
- LinkedIn People-section selector set and its live-smoke fixture (capture only with the opt-in enabled).
- Whether an unnamed generic-mailbox contact is stored (low-confidence, unnamed sentinel handling) or dropped — data-model.md permits either; pick one in implementation and test it.
