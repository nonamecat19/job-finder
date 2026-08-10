# Quickstart: Resume Generation Workspace

How to build, run and verify 042. Commands are the repo's `make` targets — the canonical entry
points, so CI and local runs stay aligned.

---

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # dashboard + api tooling depend on this first
make up                                  # Postgres, Redis, Ollama, gateway
```

## The build loop for this feature

A change here typically spans three layers. The order matters — a DTO edit that skips
`tygo-generate` fails CI on `make tygo-check`, not on your machine.

```bash
# 1. schema
#    edit apps/api/internal/db/queries/generationrun.sql
make sqlc-generate
#    migrations run on boot; goose versions must be unique and sequential (00042, 00043)

# 2. wire types
#    edit apps/api/internal/dto/generation_workspace.go
make tygo-generate
pnpm --filter @job-finder/shared build

# 3. verify
make test-lint            # both suites — required, this change crosses app boundaries
```

## Running the workspace

```bash
make dev
# dashboard http://localhost:5173 → /generate
```

Seed data with `make seed` if the profile has no master content — a run against an empty profile
is a deliberate `400`, not a bug.

---

## Verifying the three claims that matter

### SC-001 — no fabricated content in the profile-sourced group

The strongest check is structural, not empirical: grep for a text field on the ranking path.

```bash
rg 'rephrased|Rephrased' apps/api/internal/generation/
```

Nothing on the workspace path may match. `RankedSelection` carries `[]int` and nothing else, so
"a profile-sourced item differs from the master" has no representation. The API-level assertion
is the contract test on `GET /v1/generations/{runId}`: every item with `origin: "profile"` has
`text` byte-identical to the master bullet at its `sourceIndex`.

### SC-003 — 2N shown, N selected

```bash
go test ./internal/generation/domain/ -run TestVerifyRanking
go test ./internal/generation/application/ -run TestEvalCorpus -eval.case ranked-oversized-entry
```

With `experienceBulletsMin = 8` (the shipped default), an entry with ≥16 bullets shows 16 ranked
candidates with the top 8 selected, and the remainder below in master order.

### SC-004 — zero AI suggestions in an untouched export

```bash
pnpm --filter dashboard test -- GenerateWorkspacePage
go test ./internal/generation/application/ -run TestWorkspaceExport
```

The Go test asserts the assembled document contains no `origin='ai'` item that was never toggled;
the vitest asserts the UI never ships one selected.

---

## Manual walkthrough

1. `/generate`, paste a vacancy, run.
2. Left pane shows Summary, one block per master experience entry, and Skills.
3. In any work block: profile bullets ranked, top N checked, the rest unchecked below, each
   badged "from your profile". AI suggestions in their own group below, all unchecked, badged
   "AI · unverified".
4. Toggle an item — the preview updates with no spinner and no model call (SC-006).
5. Navigate away, come back — selections are as you left them (FR-020).
6. Export. If it fits you get a PDF; if not you get "3 pages, target 2" and a ranked list of the
   least relevant selected items, and **nothing is dropped for you** (FR-019).

---

## Gotchas found while planning

- **`internal/generation/singlepage/` is empty.** The chromedp density-ladder fitter described in
  `specs/domains/resume-generation.md` §7.1 does not exist. Page measurement is
  `infrastructure.CountPages`; do not plan around the fitter.
- **`internal/tailoring/` is not wired.** The `/api/tailoring` surface in §4.1 of the same
  document has no handler and no service. Migration `00043` drops its unused tables — see
  `research.md` R8, which flags this for explicit approval before it is run.
- **Grounding level means less on this route.** With no rewording possible, `strict` / `moderate`
  / `aggressive` govern the summary only. The workspace control must say so; the legacy
  `/documents/tailor` path keeps today's meaning.
- **Bumping `ScorerSetVersion` re-records every baseline.** That is correct behaviour — the
  harness refuses to compare scores measured with different instruments. Re-record per case with
  a stated reason, never for the whole corpus in one command.
- **`make test-lint` is the gate, not `go test`.** This change touches `apps/api`,
  `apps/dashboard` and `packages/shared`, so Constitution IV requires both suites.
