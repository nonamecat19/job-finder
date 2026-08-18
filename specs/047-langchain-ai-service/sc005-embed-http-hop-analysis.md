# T129: `embed`'s HTTP-hop cost against SC-005

**Status**: measurement/report only — T108's cutover is not gated on this file; a human decides
what to do with the finding. Written before `embed`'s Go path (`internal/platform/llm/
infrastructure/gateway/gateway.go`'s `Provider.Embed`) is removed, per C2-2/T129.

## What changes

- **Pre-migration**: `apps/api` → LiteLLM gateway (`/embeddings`) → provider. One network hop.
- **Post-migration**: `apps/api` → `apps/ai` (`POST /v1/capabilities/embed/invoke`) → LiteLLM
  gateway (`/embeddings`) → provider → back through `apps/ai` → back to `apps/api`. The
  gateway-to-provider leg is unchanged; the entire added cost is the new `apps/api ↔ apps/ai`
  round trip plus whatever work `apps/ai` does around it (auth check, Pydantic validation,
  Langfuse span).

SC-005: end-to-end completion time for a migrated capability must grow **no more than 10% at
the median** versus its pre-migration measurement.

## What I could and couldn't measure

I don't have a live `apps/ai` + `apps/api` + LiteLLM stack running in this sandbox, and no
recorded pre-migration latency baseline for `embed` exists yet in this repo (only `ghost`'s
accuracy baseline — `baseline-ghost-comparison.json` — exists, and it measures SC-004 score
deviation, not SC-005 timing). So this is a **structural estimate**, not a production
measurement, built from two things I could measure directly in-sandbox:

1. **Bare HTTP round-trip over loopback.** A minimal FastAPI app serving the exact
   `/v1/capabilities/embed/invoke` response shape, hit 500 times over a keep-alive `httpx`
   client after warmup:
   - median **2.0 ms**, p90 2.24 ms, p99 2.60 ms, min 1.25 ms, max 5.76 ms.
   - This is a best case: same host, no Docker bridge network, no auth middleware, no Langfuse
     span creation, no TLS.
2. **Pydantic validate/serialize round trip for a 1024-float vector** (`EmbedResult` →
   `model_dump` → `json.dumps` → `json.loads` → `EmbedResult.model_validate`, 2000 iterations):
   median **0.19 ms**, p99 0.24 ms. Negligible — the 1024-dim payload (~15–20 KB as JSON) is not
   where the cost is.

Neither measurement includes: the Docker bridge hop between `api` and `ai` containers (K4
wiring), the `AI_SERVICE_TOKEN` auth check (H7-1, not yet implemented as of this writing),
Langfuse `start_as_current_observation` span creation (T068/T070), or connection-pool
cold-starts if `internal/aiclient` doesn't keep a warm connection per worker.

## Reasoned estimate

Added latency ≈ (`apps/api`→`apps/ai` hop) + (`apps/ai` request handling) + (`apps/ai`→`apps/api`
response), where the gateway/provider leg is identical in both paths and cancels out of the
comparison.

- Loopback HTTP alone: ~2 ms median (measured).
- Docker bridge network, same host, no TLS: typically adds sub-millisecond to a few ms over pure
  loopback in practice — not measured here, called out as an assumption.
- Auth check + Pydantic input validation: sub-millisecond (measured proxy above for output;
  input validation is a simple one-field model, cheaper still).
- Langfuse span creation: the SDK batches and exports off-thread (`tracing.py`'s own docstring),
  so the synchronous cost of opening a span should also be sub-millisecond, but this is
  unverified — I did not benchmark it.

**Best-case total added overhead: on the order of 3–8 ms at the median.**

## Why this is a real risk for `embed` specifically, even though the number sounds small

SC-005's threshold is **relative** (10% of the pre-migration median), and `embed`'s
pre-migration median is the fastest call in the whole capability table — it is a single
embeddings call with no prompt, no structured-output parsing, no retries (C2-1, C2-2:
"the highest-volume call in the platform"). Typical OpenAI-compatible embedding endpoints for
short/medium text return in roughly 20–150 ms depending on provider and network path; some
self-hosted/local embedding models are faster still. Contrast this with `ghost` or `salary`,
whose pre-migration medians are hundreds of ms to seconds (a full chat completion, possibly with
tool rounds) — for those, a 3–8 ms hop is comfortably inside 10%.

For `embed`, run the arithmetic against a few plausible baselines:

| Assumed pre-migration median | 10% budget | Added hop (best case, 3 ms) | Added hop (pessimistic, 8 ms) |
|---|---|---|---|
| 150 ms | 15 ms | within budget | within budget |
| 60 ms | 6 ms | within budget (tight) | **breaches** |
| 30 ms | 3 ms | **at the boundary** | **breaches** |

I do not know `embed`'s actual pre-migration median in this environment — that number has to
come from a real measurement against the running stack (LiteLLM gateway + whichever embedding
provider is configured), not from this sandbox. The point this table makes is structural: unlike
every other capability in this migration, `embed`'s baseline is plausibly fast enough that a
single-digit-millisecond added hop is not automatically safe. It is the one capability in the
table where "the hop is small" and "the hop breaches SC-005" are both live possibilities
depending on what the real baseline turns out to be.

## Recommendation

Before T108's cutover is flipped in production (`AI_CAPABILITY_ROUTING=python` for `embed`),
record `embed`'s actual pre-migration median from the running stack, then repeat this
measurement against the real `apps/api → apps/ai → gateway` path (not the loopback proxy above)
and compare. If the real added-hop cost lands inside 10% of the real baseline, T108 stands as
implemented. If it doesn't, per C2-2 and this task's own instruction, that is **a spec question
about FR-019a's totality** — whether "every capability including embeddings" (FR-019a) is worth
relaxing for this one highest-volume/lowest-benefit case — not something to quietly relax by
loosening SC-005 or skipping the measurement. I have not made that call; flagging it here for a
human decision, and not skipping or deferring `embed`'s migration unilaterally.
