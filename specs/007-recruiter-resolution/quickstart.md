# Quickstart: Validating Recruiter / Hiring-Manager Resolution

How to prove the feature works end-to-end once implemented. Implementation code belongs in `tasks.md`, not here. **Implementation is gated on plan 004** (`internal/companyintel`) — the company-page level below assumes it has landed.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # must precede other workspace packages
make up                                   # Postgres + Redis + Ollama via Docker Compose
```

The `LINKEDIN_SCRAPE_ENABLED` env var gates the LinkedIn source. **It defaults to `false`.** Leave it unset for everything except Level 2's opt-in step.

## Capture test fixtures

Unit tests parse saved text/HTML, so no network is needed in CI:

- **Posting-text fixtures**: plain job-description strings — one naming a contact (`Contact: Jane Doe, Recruiter <jane@acme.com>`), one naming no one, one with only a generic mailbox (`jobs@acme.com`), one in Cyrillic. These are inline test strings, not files.
- **Company-page fixture**: save one company About/Team page's HTML to the recruiter package's `testdata/` (respecting the shared fetch pacing). Note the capture date in a comment.
- **LinkedIn fixture** (opt-in only): save a public company-page People section's HTML to `testdata/` **only if you run Level 2's opt-in step**. Do not commit a fixture obtained while logged in.

## Level 1 — Unit tests (no network)

```bash
cd apps/api
go test ./internal/recruiter/ -v
```

Expected:

- **Posting parser**: the contact-naming fixture yields a `ResolvedContact` with name `Jane Doe`, title `Recruiter`, email `jane@acme.com`, `Source == "posting"` (SC-001).
- **No-fabrication**: the no-contact fixture yields zero contacts and no error (FR-016, SC-002). The generic-mailbox fixture yields **no named contact** — dropped or explicitly unnamed low-confidence, never "jobs" as a person (FR-007, SC-003).
- **Field-traceability**: a fixture with a phone but no name yields no name-bearing row; no field is populated that is absent from the input (FR-008).
- **Cyrillic**: a Cyrillic contact line round-trips byte-identical.
- **Company-page parser**: the saved team-page fixture yields ≥1 contact with `Source == "company-page"`; a fixture with no People/Team section yields zero contacts and no error (edge case).
- **Confidence ordering**: given contacts from two sources, the highest-confidence one sorts first, with the deterministic tie-break (FR-010).

## Level 2 — Live smoke (network, opt-in)

Behind the existing `live` build tag:

```bash
cd apps/api

# Company-page source (needs a real Company.website from plan 004):
go test -tags live ./internal/recruiter/ -run TestLive_CompanyPage -v

# LinkedIn source — ONLY with the opt-in explicitly enabled:
LINKEDIN_SCRAPE_ENABLED=true go test -tags live ./internal/recruiter/ -run TestLive_LinkedIn -v
```

Expected: each live parser returns ≥0 contacts against a real page and logs the count — the canary for markup drift. **Confirm that with `LINKEDIN_SCRAPE_ENABLED` unset, `TestLive_LinkedIn` makes zero LinkedIn requests / is skipped** (SC-004).

## Level 3 — End-to-end through the running stack

```bash
make seed
make dev
```

Then in the dashboard (`http://localhost:5173`), with `LINKEDIN_SCRAPE_ENABLED` unset (default off):

1. **Open a job whose description names a contact** → the **Contact** line shows the highest-confidence contact's name and title, plus email/phone/LinkedIn when resolved (Story 1, SC-001).
2. **Open a job that names no one** → the Contact line shows "No contact found — try Refresh", not a blank (Story 1 scenario 2, SC-002).
3. **Click Refresh contacts** on a job whose company has a `website` → the company-page source runs and any resolved contacts appear; the line updates without a page reload (Story 2, SC-005). Confirm no LinkedIn request is made (env off).
4. **Expand the Contact line** on a multi-contact job → every stored contact is listed with source and confidence, ordered best-first (Story 3, SC-008, SC-010).
5. **Re-run Refresh twice** on unchanged data → no duplicate contacts added (FR-013, SC-006).
6. **Enable LinkedIn** (`LINKEDIN_SCRAPE_ENABLED=true`, restart) → Refresh now also scrapes the public LinkedIn company page; new rows appear with `source='linkedin'` (Story 2 scenario 3).

## Level 4 — Failure-mode checks

| Scenario | How to force it | Expected |
|---|---|---|
| No contact anywhere (FR-016) | Job with no contact in body, no company website, LinkedIn off | Resolution succeeds, zero rows, detail line = "No contact found — try Refresh" |
| Generic mailbox only (FR-007, SC-003) | Posting body has only `jobs@acme.com` | No named contact; headline falls back to "No contact found" |
| LinkedIn disabled (FR-004, SC-004) | `LINKEDIN_SCRAPE_ENABLED` unset, run Refresh | LinkedIn source silently skipped, zero LinkedIn requests, run not marked failed |
| One source fails (FR-015, SC-007) | Block the company website host (`/etc/hosts`) during Refresh | company-page source fails alone; posting contacts still persisted and shown |
| No People section (edge case) | Point company-page parser at a page with no Team section | Zero company-page contacts, no error |
| Job deleted (FR-014, SC-009) | Delete a job that has contacts | Its `JobContact` rows cascade-deleted; no orphans (verify via query) |
| Re-run idempotent (FR-013, SC-006) | Refresh the same job twice, unchanged data | Row count unchanged after the second run |

## Regression gate

```bash
cd apps/api && go test ./...          # unit + Docker-backed integration (cascade, upsert)
pnpm --filter @job-finder/dashboard test   # vitest for the Contact-line UI
make test-lint                         # boundary gate — change spans apps/api + apps/dashboard
```

The change crosses `apps/api` and `apps/dashboard`, so per Constitution Principle IV `make test-lint` is the binding merge gate, not just `go test`.

**Constitution Principle I check**: grep the recruiter package for any non-GET request or any message/email/apply call against a resolved contact. There must be none — resolution is read-only; outreach is out of scope.

**Constitution Principle II check**: confirm the no-fabrication tests (Level 1) are present and passing — they are what makes the LLM source constitutionally acceptable.
