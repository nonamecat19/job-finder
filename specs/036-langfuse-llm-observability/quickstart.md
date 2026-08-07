# Quickstart: Self-Hosted LLM Observability

**Feature**: `036-langfuse-llm-observability` | **Date**: 2026-08-07
**Revised**: 2026-08-07 after audit. Several steps previously asserted fields and checks that do not
work. Corrections are marked inline.

Ten scenarios. Steps 1–4 validate US1, step 5 is the safety gate, steps 6–7 the privacy boundary,
steps 8–9 US2, step 10 the unconfigured path. Run them in order.

---

## 0. Prerequisites

```bash
cp .env.example .env            # if not already present
# fill: LITELLM_MASTER_KEY, at least one provider key, OLLAMA_URL/OLLAMA_KEY
# fill: LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY, LANGFUSE_HOST,
#       LANGFUSE_DB_URL, LANGFUSE_CLICKHOUSE_URL, LANGFUSE_REDIS_URL,
#       LANGFUSE_S3_*, NEXTAUTH_SECRET, NEXTAUTH_URL, SALT, AUTH_DISABLE_SIGNUP
make up
```

**Note (C2-6)**: `LANGFUSE_HOST` is a *container-network* address
(`http://langfuse-web:3000`). Verifying from the host uses the published port instead — the previous
version of this document curled `$LANGFUSE_HOST` from the host, which cannot resolve. The same trap
is already documented for `GATEWAY_URL` in `.env.example`.

```bash
curl -sf http://localhost:3001/api/public/health && echo "collector ok"   # host port
curl -sf http://localhost:4000/health/liveliness && echo "proxy ok"
```

---

## 1. Every call produces a record (FR-001, FR-002, SC-001)

```bash
make seed
# then drive one match and one full tailoring pass
```

**Expect** one record per AI call, each carrying:

| Field | Expected value |
|---|---|
| `metadata.model_group` | the requested task key (`generation-summary`, `match`, …) |
| `model` | the **served deployment** (`openrouter/anthropic/claude-sonnet-5`) |
| `name` | `litellm-acompletion` — **not** the task key, until US2 sends `generation_name` |
| `usage.input` / `usage.output` | token counts |
| `usage.total_cost` | see step 3's caveat |
| `startTime` / `endTime` | duration |

**Corrected**: the previous version expected `name` = the task key and a `metadata.served_model`
field. Neither exists. The task key lives in `metadata.model_group`; there is no `served_model`, because
the callback receives `kwargs`, not the HTTP response, and drops the `headers` key before logging.

**SC-001 counting caveat**: the platform's activity steps are **not** one-per-AI-call —
`CompleteStructured` retries produce no step. Count against the proxy's own request log, not activity
steps, or the comparison cannot agree exactly.

---

## 2. A fallback is visible as a fallback (FR-003)

Force the primary tier of one task to fail:

```bash
$EDITOR gateway/config.yaml     # point generation-summary tier 1 at an unreachable api_base
docker compose restart litellm
```

Run one tailoring pass.

**Expect**: `metadata.model_group` = `generation-summary` while `model` names a *different*
deployment. The two differing is the whole signal.

**Corrected**: previously written as `model` vs `metadata.served_model`. The comparison is
`metadata.model_group` (requested) vs `model` (served).

Restore and restart before continuing.

---

## 3. Local-tier calls (FR-014, C6-1)

Drive a task whose chain exhausts to `local`.

**Expect**: a record exists and `model` names the local deployment.

**Do not assert cost == 0 blindly.** The proxy writes no cost for a deployment absent from its cost
map, so free and unpriced are indistinguishable. Either `local` carries an explicit zero cost in
`gateway/config.yaml` — in which case assert 0 — or FR-014 is weakened to "a record exists" (C6-2).
Decide which before running this step; the previous version asserted "0, not null" against data that
cannot support it.

---

## 4. Config-only is really config-only (SC-002)

**Corrected**: the previous version ran `git status --porcelain apps/api`, which inspects the working
tree. That tree already carries unrelated changes and cannot distinguish this feature's edits.

```bash
git diff --stat <feature-base-commit>..HEAD -- apps/api
```

**Expect**: empty for everything validated in steps 1–3. Allowed to have changed:
`docker-compose*.yml`, `gateway/config.yaml`, `.env.example`, `specs/domains/*`.

---

## 5. A dead collector cannot break inference (FR-004, SC-003) — **the gate**

The scenario this feature is not allowed to fail.

**First, produce the baseline SC-003 actually needs.** Measure with callbacks *disabled*:

```bash
$EDITOR gateway/config.yaml     # success_callback: []  failure_callback: []
docker compose restart litellm
# drive >= 20 calls on one task key; record proxy-side durations
```

Then re-enable callbacks and repeat. **Expect**: the two distributions are indistinguishable at the
median.

**Corrected**: the previous version compared collector-up against collector-down, both with callbacks
configured, and asserted a 50 ms bar. That measures the wrong thing — it never isolates latency
*attributable to observability* — and 50 ms is unresolvable against per-call variance measured in
seconds.

Then the failure modes:

```bash
docker compose stop langfuse-web langfuse-worker
```

Run a full tailoring pass and a match. **Expect**: both complete, zero failed calls, no measurable
slowdown, the proxy logs a delivery failure and continues without retrying in the request path.

Then the harder variant — reachable but not answering:

```bash
docker compose pause langfuse-web
```

Same expectations. Unpause afterwards.

**Fails if**: any call fails, hangs, or slows measurably. A failure here blocks the feature.

---

## 6. Nothing leaves the deployment (FR-005, SC-006)

**Corrected**: the previous version grepped for `telemetry|cloud|host`, guessing at variable names.
Check the collector's named telemetry setting explicitly instead.

```bash
docker compose exec langfuse-web env | grep -iE 'TELEMETRY_ENABLED|LANGFUSE_CLOUD'
grep -n "LANGFUSE_HOST\|NEXTAUTH_URL" .env
```

**Expect**: telemetry explicitly disabled; both URLs deployment-internal (C2-1); signup disabled
(C2-5).

---

## 7. Credentials are not readable from the application (FR-007, SC-009)

```bash
docker compose exec api env | grep -i 'LANGFUSE\|API_KEY\|MASTER_KEY'
```

**Expect**: no output.

**This will fail before the feature's credential work is done.** `docker-compose.prod.yml:27` gives
`api` the whole `.env` via `env_file`, so it currently holds every provider key — a standing
violation of Principle V's credential clause, independent of this feature. C2-2 requires narrowing
that service to an explicit environment list. Note also that dev has no `api` service at all (the Go
process runs on the host reading the same `.env`), so this check is dev-unenforceable by
construction; state that rather than pretending otherwise.

---

## 8. One run reads as one story (FR-009, FR-010, FR-011, SC-005) — US2

Run a single tailoring pass.

**Expect**: one trace containing every call that pass made, in order, with run-level totals, and each
record's `name` now the task key (`generation_name` is being sent). Retries and escalations appear
*inside* the trace.

**Verify the key**: the metadata must be `existing_trace_id`. If `trace_id` was used, the trace's
`name`/`input`/`output` will describe whichever call finished last — check those fields, not just
that grouping happened, because grouping works either way.

Cross-reference (FR-010): the trace id equals the activity-run id in the platform's own history.

Then concurrency (FR-011, SC-005): trigger 10 concurrent runs. **Expect** ten traces, zero
cross-attribution.

---

## 9. Reporting groups by task, not by model (FR-012, SC-004, SC-004a)

```bash
# in the collector UI: group cost by generation name / tag over the last 7 days
```

**Expect**: `generation-summary` and `generation-select-premium` appear as **separate** buckets.

**This is the check that would have caught the original design error.** Both are served by the same
model, so grouping by `model` collapses them into one — erasing exactly the per-stage distinction
feature 035 exists to create. If they appear as one bucket, `generation_name`/`tags` are not being
sent and FR-012 is unmet regardless of what the per-call records look like.

---

## 10. Retention is proven by deletion (FR-008, SC-008)

```bash
# write a record with a timestamp older than EVAL_PRUNE_RETENTION_DAYS
# run the pruning job
```

**Expect**: the record is gone, and the job logged what it deleted (FR-008a).

**Corrected**: there is no `LANGFUSE_RETENTION_DAYS`. Automated retention is enterprise-only; this
window is enforced by a pruning job the platform owns (research R7). Reading a configuration value
proves nothing.

---

## 11. Unconfigured is a supported state (FR-015, SC-007)

```bash
docker compose stop langfuse-web langfuse-worker clickhouse
# comment out every LANGFUSE_* in .env
docker compose restart litellm
make test
```

**Expect**: stack up, every AI task served, `go test ./...` passes.

---

## Success summary

| Step | Requirement | Criterion |
|---|---|---|
| 1 | FR-001, FR-002 | SC-001 — record per call, counted against the proxy log |
| 2 | FR-003 | `model_group` ≠ `model` when a fallback answers |
| 3 | FR-014 | record exists; zero-cost only if `local` is priced (C6-2) |
| 4 | FR-006 | SC-002 — clean `apps/api` diff vs the base commit |
| 5 | FR-004 | SC-003 — callbacks-on vs callbacks-off indistinguishable; dead **and** hung collector |
| 6 | FR-005 | SC-006 — telemetry off, URLs internal, signup disabled |
| 7 | FR-007 | SC-009 — no credential in the app container |
| 8 | FR-009–FR-011 | SC-005 — `existing_trace_id`, 10 concurrent, zero cross-attribution |
| 9 | FR-012 | SC-004a — two task keys on one model stay two buckets |
| 10 | FR-008 | SC-008 — proven by deletion |
| 11 | FR-015 | SC-007 — suites pass with nothing configured |
