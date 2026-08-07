# Quickstart: Validating Split-Model Resume Generation

Runnable checks that prove the feature works end to end. Each maps to a spec requirement.

## Prerequisites

```bash
make up                                   # postgres, redis, ollama, litellm, minio
pnpm install && pnpm --filter @job-finder/shared build
cd apps/api && go run ./cmd/server        # or `make dev`
```

A default profile with a populated `rendercvConfig` must exist (`make seed` provides one). The
vacancy fixtures used during evaluation are in `resume_test/vacancies/`.

## 1. Each stage routes to its own task key (FR-002, FR-003)

```bash
curl -sS localhost:3000/api/documents/tailor \
  -H 'content-type: application/json' \
  -d @resume_test/vacancies/tailor-request.json | jq '.resume | {summaryModel, model, stageCostUsd}'
```

Then read the API log:

```bash
grep 'gateway request' <api log> | tail -5
```

**Expected**: one line per stage with `requested_group=generation-analyze`,
`generation-select`, `generation-summary` — never a provider or model name in the request. The
served models differ per stage: economy for analyze/select, premium for summary.

**Fails if**: any stage requests `generation`, or the same served model appears for every stage.

## 2. Cost and latency targets (SC-001, SC-002)

```bash
cd apps/api && go test ./internal/generation/application -run TestGenerationBenchmark -v
```

**Expected**: median per-resume cost ≤ ⅕ of the recorded pre-split baseline ($0.113) and median wall
clock ≤ ½ of it (60s). Target figures from evaluation: ~$0.011, ~20s.

**Note**: cost comes from the gateway's `usage.cost` captured per stage (FR-017). If
`stageCostUsd` is null on the document, cost capture is not wired and this check cannot pass.

## 3. Truncated selection is caught (FR-006, FR-007, US2)

Unit-level, no network:

```bash
cd apps/api && go test ./internal/generation/domain -run TestVerifyCompleteness -v
```

**Expected**: a merged document missing a master skill that the analysis lists as **required**
reports `Shortfall` with that token in `RequiredMissing`; retaining 79% of nice-to-have matches
reports a shortfall, 80% does not; a company below `ExperienceBulletsMin` appears in
`BulletShortfalls`; an analysis with no required skills sets `StructuralFallback`.

End-to-end escalation:

```bash
cd apps/api && go test ./internal/generation/application -run TestSelectionEscalatesOnRepeatShortfall -v
```

**Expected**: a fake provider returning truncated selection twice causes a third call on the
premium router, the run completes, and the document has `selectionEscalated = true`.

**Fails if**: the truncated document renders, or the run fails instead of escalating.

## 4. The summary is immutable after page fitting (FR-010)

```bash
cd apps/api && go test ./internal/generation/application -run TestPageFitDoesNotAlterSummary -v
```

**Expected**: with a shape config forcing an expand and a fake page-fit provider that attempts to
return a different summary, the rendered document's summary is byte-identical to the premium
stage's output. The `TailoredSelection` schema has no summary field, so the attempt is discarded at
unmarshal.

## 5. Premium outage is visible, not silent (FR-011, FR-012, SC-003)

Force the summary chain's tier 1 to fail by pointing it at an unreachable deployment:

```bash
# edit gateway/config.yaml: generation-summary -> a bogus model id
docker compose restart litellm
curl -sS localhost:3000/api/documents/tailor -H 'content-type: application/json' \
  -d @resume_test/vacancies/tailor-request.json | jq '.resume | {summaryModel, summarySubstituted}'
```

**Expected**: the run completes (chain falls through, ultimately to local Ollama),
`summarySubstituted` is true, `summaryModel` names the fallback, and `TailorPage` renders the
substitution marker on the result surface.

**Restore afterwards**: revert `gateway/config.yaml`, `docker compose restart litellm`.

## 6. Full local-only operation (FR-011, SC-008, Constitution V)

```bash
GATEWAY_URL= go run ./cmd/server        # gateway unconfigured
```

Run the tailor request again. **Expected**: every stage routes to Ollama and a resume is still
produced. No stage errors on the missing gateway.

## 7. No cover letter unless asked (FR-013, US4, SC-010)

```bash
curl -sS localhost:3000/api/documents/tailor -H 'content-type: application/json' \
  -d @resume_test/vacancies/tailor-request.json | jq '.coverLetter'      # -> null

RESUME_ID=$(… id from the response …)
curl -sS -X POST localhost:3000/api/documents/$RESUME_ID/cover-letter | jq '{id, type}'
```

**Expected**: the tailor response has no cover letter and is faster than the pre-split run; the
explicit request returns a cover letter stored against the same job. Job-triggered generation
(enqueue a `generate` task) likewise produces a resume only.

## 8. Full suite before calling it done (Constitution IV)

```bash
make test-lint            # go test + vitest, both apps
make test-integration     # real Postgres/Redis
```

This feature touches `apps/api`, `apps/dashboard` and `packages/shared`, so `make test-lint` is
required, not optional.
