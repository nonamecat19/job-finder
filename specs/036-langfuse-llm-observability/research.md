# Phase 0 Research: Self-Hosted LLM Observability

**Feature**: `036-langfuse-llm-observability` | **Date**: 2026-08-07

Seven decisions. All resolved; no NEEDS CLARIFICATION remains.

---

## R1 — Collector: self-hosted Langfuse, not a hosted SaaS

**Decision**: Deploy Langfuse as a container in the existing Compose stack. Never point the callback
at `cloud.langfuse.com`.

**Rationale**: Constitution Principle V forbids core inference depending on third-party services, and
prompts here carry the user's master profile — real name, employers, contact details, dates. A hosted
collector would ship that PII off-box on every generation. Self-hosted keeps it inside the same trust
boundary as the Postgres row it came from.

**Alternatives rejected**:
- *Langfuse Cloud*: violates Principle V and FR-005 outright.
- *OpenTelemetry + a generic backend (Tempo/Jaeger)*: correct for spans, poor for LLM specifics —
  no first-class prompt/completion viewer, no cost aggregation, and the proxy's OTel exporter carries
  less LLM metadata than its Langfuse integration.
- *Roll our own table in the platform's Postgres*: cheapest to start, but then FR-012's reporting is
  our problem to build and maintain, which is exactly the work this feature exists to avoid.

---

## R2 — Langfuse major version: v4, and the infrastructure cost is accepted

**Decision**: Pin Langfuse **v4** — web + async worker, ClickHouse, Redis, and an S3-compatible blob
store.

**Rationale**: This decision was made once on infrastructure weight alone and was wrong. The
corrected reasoning:

| Option | Weight | Currency | Verdict |
|---|---|---|---|
| v2 | Lightest — one container on Postgres | **Two majors behind.** v4 is GA as of 2026; the maintained path is v3→v4, and legacy ingestion paths are being removed | Rejected |
| v3 | ClickHouse + Redis + blob store | Superseded by v4 | Rejected — same infra cost as v4 with a shorter horizon |
| **v4** | ClickHouse + Redis + blob store | Current and supported | **Chosen** |

The infrastructure delta is smaller than it first appears: the deployment already runs Redis and
MinIO (S3-compatible), so the genuinely new stateful service is **ClickHouse**. That is a real cost —
a new memory floor and a new backup surface for a deployment whose trace volume is a few hundred
calls a day — and it is accepted deliberately rather than traded away for a version that will need
redoing.

**What the original v2 decision got wrong**, recorded so the mistake is not repeated: it weighed
container count and never asked (a) how old v2 was, or (b) whether v2 could satisfy the requirements
that depended on it. It could not — see R7, where the retention mechanism this spec leaned on turned
out not to exist for any OSS self-host.

**Alternatives rejected**:
- *v2 or v3*: see table.
- *Reusing the platform's own Postgres instance for Langfuse's metadata schema*: a separate logical
  database on the same server is acceptable; sharing a schema entangles the platform's goose
  migrations (sequential versions per Constitution) with a vendor's. ClickHouse is separate by
  construction.
- *Dropping Langfuse for a traces table in the platform's own schema*: genuinely viable and much
  lighter — retention becomes a `DELETE`, nothing leaves the box, no vendor. Rejected because
  FR-012's reporting then becomes SQL this project writes and maintains, and the "zero Go change"
  property of US1 disappears entirely. Worth revisiting if ClickHouse proves burdensome in operation.

**Open item folded into the plan**: pin the exact image tags at implementation time and record them,
the way `ghcr.io/berriai/litellm:main-stable` is recorded today.

---

## R3 — Wiring: proxy success/failure callbacks, not application instrumentation

**Decision**: Enable the collector through the routing service's callback configuration
(`litellm_settings.success_callback` / `failure_callback`) with credentials injected as environment
variables into that container.

**Rationale**: This is what makes US1 config-only. The proxy already sees every request, already
resolves the task key to a serving model, already computes cost, and already knows which tier
answered. Instrumenting the Go client would duplicate all of that and would have to be repeated at
every call site.

**Consequence for FR-013 (coverage)**: anything that does not traverse the proxy produces no record.
Verified against the tree:

- **Embeddings — never, unconditionally.** The first version of this section said "when `EMBED_URL`
  points directly at a local Ollama", implying a configuration that could route them through the
  proxy. There is none: `Router.Embed` delegates straight to the local provider
  (`application/router.go:71-73`), and even the gateway provider's `Embed` forwards to Ollama. This
  is already binding in `specs/domains/llm-routing.md` (030-FR-014). Two live call sites are
  therefore permanently uncovered and must be named rather than implied:
  `matching/application/service.go` and `profile/application/service.go`.
- **The local-provider path when `GATEWAY_URL` is unset** — Principle V's local-first guarantee
  working as designed, and by construction invisible to a proxy that is not running.

Both are documented as out of coverage rather than papered over. FR-014's "local calls produce a
zero-cost record" applies to the `local` tier *reached through the proxy*, which is the common case.

**Evidence for R4's isolation claim, found while verifying this**: `litellm_logging.py` dispatches
the Langfuse callback on an executor rather than in the request path, with the comment that these
"are sync callbacks. We need to call them in the executor". That is real support for FR-004 which
the original research asserted without citing.

**Alternatives rejected**:
- *Wrap the Go `Provider` interface with a tracing decorator*: gives coverage of the two gaps above,
  but costs a Go change for the MVP and duplicates cost/served-model logic the proxy already owns.
  Reconsider only if the coverage gaps ever matter.

---

## R4 — Failure isolation: the callback must not be able to break inference

**Decision**: Treat trace delivery as best-effort. Verify explicitly that a dead collector does not
fail or delay calls, and keep the verification as a quickstart scenario rather than trusting the
integration's documentation.

**Rationale**: FR-004 and SC-003 are the load-bearing safety requirements of this feature. An
observability layer that can take down generation is worse than no observability. The proxy's
callback dispatch is asynchronous, but "asynchronous" has failed before in this stack — 030's
`request_timeout` was documented as enforced and observed not to be, and one call hung for 830
seconds as a result. The same skepticism applies here: prove it with a dead collector, don't assume.

**Alternatives rejected**: trusting the upstream's stated behaviour without a test. Explicitly
rejected on the 830-second precedent.

---

## R5 — Run correlation: application-supplied metadata (US2 only)

**Decision**: Group a run by passing a correlation identifier as request metadata from the
application, reusing the platform's existing activity-run identifier as the value, sent as
**`existing_trace_id`**.

**The key name matters and the obvious one is wrong.** `metadata.trace_id` is for *creating* a trace;
sending it on every call of a run makes each call rewrite the trace's `name`, `input`, `output` and
`tags`, so trace-level fields end up describing whichever call finished last. The integration's own
source carries the warning ("DO NOT SET TRACE_NAME if trace-id set. this can lead to overwriting of
past traces"). `existing_trace_id` is the append-without-overwrite key and is what FR-009 needs.

**Scope correction**: the calls needing this are **not** in
`generation/application/service.go`, as the first version of this research assumed. They are six free
functions in `generation/application/rendercv_llm.go` — `analyzeVacancy` (:68), `selectContent`
(:299), `writeSummary` (:352), the structure re-tailor (:372), `expandContent` (:386) and
`condenseContent` (:476) — plus the cover-letter call in `service.go`. None receives an
`*activity.Recorder`, so US2 is a signature change across seven call sites, not "thread a field
through one file".

**Rationale**: The proxy cannot invent the grouping — only the application knows that three calls
belong to one tailoring pass. The request body's metadata field is the transport, and the existing
activity-run id is the natural value because it makes FR-010's cross-reference free: an operator
looking at a trace can find the same id in the platform's own history and vice versa.

This is the reason US2 is honestly labelled as *not* config-only. The change is small — a metadata
field threaded through `CompleteOptions` into the gateway request body — but it is a Go change, and
the spec says so rather than overselling the feature.

**Alternatives rejected**:
- *Correlate by timestamp proximity*: fails FR-011 under concurrency, which the worker pool makes
  routine.
- *A per-run proxy virtual key*: works, but means minting and revoking keys per run, which is far
  more machinery than a metadata field.
- *Deriving the run from the task key*: task keys are per-stage, not per-run; two concurrent resume
  runs share every key.

---

## R6 — Reporting surface: the collector's own UI, but the grouping must be made, not assumed

**Decision**: FR-012's questions are answered in the collector's own interface. The per-task grouping
is created explicitly by sending `generation_name` and `tags` in request metadata — it is **not**
free.

**The premise this decision originally rested on was false.** The first version of R6 claimed that
because the application sends the task key in the `model` field, the collector's built-in per-model
grouping *is* per-task grouping "with no extra work". Verified against the pinned image, it is not:

| What the spec assumed | What the integration actually does |
|---|---|
| Langfuse `model` = the requested task key | `model` = the **served deployment** (`openrouter/anthropic/claude-sonnet-5`). The requested key survives as `metadata.model_group` |
| `metadata.served_model` from `x-litellm-model-name` | No such field. The callback receives `kwargs`, not the HTTP response, and the integration drops the `headers` key before logging |
| Record `name` = the task key | Defaults to `litellm-{call_type}` unless `generation_name` is sent |

Left uncorrected, `generation-summary` and `generation-select-premium` would collapse into one
`claude-sonnet-5` bucket — the exact per-stage distinction feature 035 exists to create.

**Corrected mechanism**:
- **Per-task grouping**: send `metadata.generation_name` = the task key, and a `metadata.tags` entry
  carrying it, so the collector's own filtering works on the task rather than the deployment.
- **Fallback rate (FR-003, FR-012)**: still derivable, but from `metadata.model_group` (requested)
  versus `model` (served) — not from a `served_model` field.

**Consequence for scope, stated plainly**: sending metadata is an application change. It is the same
change US2 already needs for its correlation id, so per-task reporting moves out of US1's
config-only claim and into US2. US1 still delivers per-call cost, latency, tokens and outcome with
zero Go changes; it does not deliver per-*task* reporting. The spec must say so.

**Alternatives rejected**:
- *A dashboard surface in `apps/dashboard`*: out of scope — observability is an operator tool.
- *Post-processing exports to recover the task key*: works, but turns "answer in under a minute"
  into an export-and-script workflow, which is not what SC-004 promises.

---

## R7 — Retention: a pruning job this project owns, because the built-in one is not available

**Decision**: 30-day retention, enforced by a **scheduled pruning job in the platform**, not by a
collector setting.

**The original decision named a mechanism that does not exist.** It specified a
`LANGFUSE_RETENTION_DAYS` environment variable. Langfuse's automated data-retention feature is
**enterprise-only**; OSS self-hosted deployments — which Principle V requires us to be — must
implement their own pruning. There is no version of this collector, v2 or v4, where the window
configures itself.

That is not a cosmetic correction. R7's whole argument for *retaining* prompt bodies rather than
redacting them was that a bounded window is the cheaper control. If the window is not free, the
comparison has to be made again rather than inherited.

**Re-decided, with the real costs**:

| Option | Cost | Verdict |
|---|---|---|
| Own a pruning job | A scheduled deletion against the collector's API or store. The platform already runs `robfig/cron`, so the scheduling machinery exists | **Chosen** |
| Redact before collection | Cheap to run, but a redacted trace cannot answer the grounding question that justifies collecting traces — the original rejection still holds | Rejected |
| Drop the guarantee | Honest, but leaves the user's employment history accumulating indefinitely, which is what FR-008 exists to prevent | Rejected |

**Why 30 days**: long enough to investigate a grounding complaint about a resume generated last
month; short enough to bound the store.

**What this adds to scope**: a pruning job, its schedule, and a test that it actually deletes. None
of these existed in the original task list — an omission that followed directly from believing the
window was a configuration setting. The domain documentation must not state a 30-day guarantee until
the job enforcing it exists, or the repository permanently carries a false binding rule.

**Alternatives rejected**: relying on ClickHouse TTLs configured by hand. Possible, but it puts the
guarantee in a place no test can see and no reviewer will find.

---

## Resolved unknowns summary

| Unknown | Resolution |
|---|---|
| Which collector | Self-hosted Langfuse (R1) |
| Which version / infra cost | **v4**; ClickHouse is the one genuinely new service, accepted (R2) |
| How wired without app changes | Proxy success/failure callbacks (R3) |
| Blast radius if collector dies | Best-effort; callbacks dispatch off the request path, still proven by a dead-collector scenario (R4) |
| How runs are grouped | Activity-run id sent as **`existing_trace_id`**; seven call sites in `rendercv_llm.go` + `service.go` (R5) |
| Where reports come from | Collector UI, but per-task grouping must be **created** via `generation_name`/`tags` — it is not free, and it moves to US2 (R6) |
| How long data is kept | 30 days, enforced by a **pruning job this project owns** — the built-in feature is enterprise-only (R7) |

## Corrections log

This research was revised on 2026-08-07 after an audit checked its claims against the tree and the
vendor's source. Four decisions were materially wrong:

1. **R2** chose v2 on container count without checking its age or whether it could meet the
   requirements resting on it. Now v4.
2. **R6** assumed the collector's `model` field carries the task key. It carries the served
   deployment. Per-task reporting is not free and is no longer claimed under US1.
3. **R7** specified a retention setting that does not exist for OSS self-hosts at any version.
   Retention is now an owned pruning job with its own tasks.
4. **R5** named `trace_id`, which overwrites; the correct key is `existing_trace_id`. It also named
   the wrong file for the call sites.

The pattern worth keeping: every one of these was a confident claim about someone else's software or
about this repository's own layout, made without opening either.
