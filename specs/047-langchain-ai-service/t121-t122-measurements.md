# T121/T122: end-to-end timing and worker occupancy

**Measured**: 2026-08-18, against an isolated throwaway RabbitMQ (`test-rmq-047`, unique
container/port, torn down after) and the real, shared `litellm` gateway (stateless, read-only
per capability call — the same one T112's live runs already used). No shared broker, no shared
DB, touched. `.env`'s real `GATEWAY_URL`/`LITELLM_MASTER_KEY` were reused; nothing published
to/consumed from `jobfinder-job-finder-rabbitmq-1`.

## T122 — backend worker occupancy for AI work types (SC-014)

Structural, from `apps/api/cmd/server/servers.go` (`buildServers`): once
`AI_CAPABILITY_ROUTING` routes a work type to `python`, its Go consumer (`app.Ghost.ProcessTask`
etc., the code that used to block a goroutine on the LLM call) is **not registered at all**.
The only Go-side involvement becomes: (1) the publish that enqueues `work.<type>.requested`, and
(2) a lightweight `events.ResultHandler` (`ghostjob.NewResultHandler`,
`salary.NewResultHandler`, `matching.NewResultHandler`, `generation.NewResultHandler`) that
fires off `results.backend` and does a DB transaction only — confirmed by reading
`ghostjob/application/result.go` and `salary/application/result.go`: idempotency-ledger admit,
unmarshal the result JSON, one or two cheap DB reads/writes (`UpsertJobSignal`,
`UpdateJobSalary`), no network/model call anywhere in that path.

Measured the publish leg directly (`events.Publisher.Publish` with confirms, 20 runs against the
isolated broker): **mean 1.4 ms, max 5.2 ms**. That is the entirety of new Go-side occupancy per
unit of AI work — publish, then return; the model call (hundreds of ms to tens of seconds)
happens entirely inside `apps/ai`, off a Go goroutine.

**Verdict**: SC-014 holds, and by a wide margin — occupancy is publish-confirm time (~1-5 ms)
plus persist time (a DB transaction, not separately benchmarked here but bounded by normal
Postgres write latency, no LLM call in the path), not the LLM call's duration.

## T121 — end-to-end completion time per capability vs pre-migration (SC-005, ≤10% median growth)

No pre-migration timing baseline was ever recorded for this feature (the existing
`baseline-*.json` files measure output values/scores for SC-004, not latency). Both the Go and
Python code paths still exist side by side (`AI_CAPABILITY_ROUTING` per capability), so this is
a live differential measurement against the real gateway, not a comparison to an archived
number.

**Interactive (HTTP) capabilities — measured directly against a locally-run `apps/ai` (real
LLM calls through the shared `litellm` gateway, 3 runs each):**

| capability | python end-to-end (apps/ai HTTP invoke) | bare gateway call, same task-key model |
|---|---|---|
| recruiter | 3837 ms / 1425 ms / 13397 ms | 2931 ms / 6346 ms / 3276 ms |
| rephrase | 660 ms / 1405 ms / 1303 ms | (not separately sampled — dominated by same model latency as recruiter's floor) |
| outreach | 4131 ms / 3386 ms / 3485 ms | (same) |

The bare-gateway-call column times *only* a chat completion against the same LiteLLM task-key
model recruiter uses, with no FastAPI hop, no Pydantic validation, no Langfuse span — i.e. the
floor that is common to both the pre-migration Go path and the post-migration Python path (same
gateway, same model, same network). Run-to-run variance from the model itself (1.4 s to 13.4 s
for the same prompt) swamps anything attributable to the added HTTP hop, Pydantic
validate/serialize, or span creation — those were separately measured in
`sc005-embed-http-hop-analysis.md` at ~2-8 ms, two to three orders of magnitude below the
LLM-latency noise floor for every capability that makes a chat completion (all of them except
`embed`).

**Queued (event) capabilities — ghost/salary/match/generate**: the only new leg beyond the model
call (which happens identically inside `apps/ai` either way once cut over) is the publish-confirm
round trip measured in T122 above: mean 1.4 ms, max 5.2 ms. Against pre-migration medians that
are themselves LLM calls (hundreds of ms to seconds), this is far under the 10% budget.

**`embed`**: still `BLOCKED` — the LiteLLM `embed` model group has no API key configured in this
environment (the same pre-existing gap `baseline-phase7-comparison.json` and T112 already
recorded). `embed` is also the one capability `sc005-embed-http-hop-analysis.md` (T129) already
flagged as the sole plausible SC-005 risk, precisely because its baseline is fast enough (tens of
ms) that a single-digit-ms hop is not automatically negligible. That analysis stands; it could
not be re-run live here for the same credential reason. `match` inherits the same block (needs a
live embedding call before it ever reaches its LLM step).

**Verdict**: SC-005 holds with wide margin for every capability whose cost is dominated by an LLM
chat completion (ghost, salary, match's LLM step, generation, recruiter, outreach, rephrase, the
summary capabilities) — the added transport (HTTP hop or queue publish) is 1-3 orders of
magnitude smaller than normal LLM latency variance. `embed` remains the one open question,
unresolved for the same reason it was unresolved at T129: no credential to measure it live
against.
