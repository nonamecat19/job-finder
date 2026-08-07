# Phase 0 Research: Split-Model Resume Generation

All findings below are from direct measurement on 2026-08-07 against the live OpenRouter
catalogue and this deployment's gateway, unless stated otherwise. Raw data:
`resume_test/round2_results.json`, `resume_test/round3.log`, `resume_test/shortlist_results.json`.

## R1: Stage-to-model assignment

**Decision**: economy stages (analyze, select, page-fit) on `google/gemini-2.5-flash-lite`;
summary on `anthropic/claude-sonnet-5`.

**Rationale**: measured on one vacancy with an identical prompt and the real strict schema, seven
economy candidates in parallel:

| model | $/call | sec | skill tokens | ungrounded | highlights | overlap |
|---|---|---|---|---|---|---|
| **gemini-2.5-flash-lite** | 0.00131 | 6.0 | 186 | 0 | 15 | 1.0 |
| qwen3.5-flash | 0.00092 | 13.1 | 187 | 0 | 15 | 0.92 |
| glm-4.7-flash | 0.00091 | 25.3 | **17** | **7** | 15 | 1.0 |
| deepseek-v4-flash | 0.00147 | 27.7 | **89** | 0 | 15 | 1.0 |
| mistral-small-3.2 | 0.00082 | 29.0 | 104 | 1 | **6** | 1.0 |
| gpt-oss-120b | 0.00041 | 43.9 | 179 | 0 | 15 | **0.65** |
| deepseek-v3.2 | 0.00367 | 190.4 | 187 | 0 | 14 | 0.94 |

All seven returned valid JSON; validity does not discriminate. Completeness does — three models
silently dropped content. gemini-2.5-flash-lite is fastest, complete, fully grounded, and 4.7x
cheaper than gemini-3-flash while returning 186 skill tokens to that model's 101.

For the summary, claude-sonnet-5 (measured $0.0414 at a full 9k-token prompt, 19.4s) is the only
premium option whose output the user has reviewed. On a trimmed summary prompt (~3k in, ~200 out)
it costs ~$0.008.

**Alternatives considered**: qwen3.5-flash for economy (cheaper, 2x slower, more rewriting —
acceptable second choice); gpt-oss-120b (cheapest by 3x but 0.65 overlap and 3 drifted highlights,
i.e. it rewrites rather than selects); claude-haiku-4.5 for summary (~$0.004/resume total, kept as
the summary chain's second tier); deepseek-v4-pro, the incumbent — slowest at 164s, uncontrollable
reasoning spend, and it needs a 32k budget, so it is excluded from every tier.

## R2: Reasoning control belongs to routing config

**Decision**: each deployment declares the switch its provider honours —
`reasoning_effort: low` (OpenAI, Anthropic), `reasoning: {enabled: false}` (z-ai, deepseek-flash).
Adding a candidate without declaring one is a configuration error.

**Rationale**: reasoning tokens count against `max_completion_tokens`. At the pre-existing 4096
cap, deepseek-v4-pro consumed the entire budget reasoning and returned empty content with
`finish_reason: length`, which surfaced as `not valid JSON: unexpected end of JSON input` after
every retry — **every resume run on the default configuration was failing**. Measured effect of
the switch on gpt-5-mini: reasoning 3328 → 448 tokens, cost halved. deepseek-v4-pro honours
neither switch (6315 reasoning tokens regardless), which is what disqualifies it.

**Alternatives considered**: raising the app-side cap alone (works, but pays for reasoning on
every call and leaves latency at 107–164s); sending the switch from application code (would put
provider-specific parameters in Go, violating the routing contract).

## R3: Prompt trimming for the summary stage

**Decision**: the summary request carries the vacancy analysis, the derived years figure, the
selected highlights and the leading skill groups — not the master profile.

**Rationale**: the master is ~17KB (~5k tokens) and dominates cost at premium rates. A summary
needs what it will reference, and after the selection stage that is a short list. ~3k prompt
tokens instead of ~9k puts the premium call at ~$0.008.

**Alternatives considered**: full master to the summary stage (3x the premium cost for context the
stage cannot use); summary before selection (cannot cite a selected achievement, which FR-004
requires).

## R4: How completeness is measured

**Decision**: tokenize the returned skill details and compare against master skill tokens
partitioned by the vacancy analysis — every master token matching `RequiredSkills` must be present
(exact), tokens matching `NiceToHaveSkills` must be ≥80% present, per-job highlight counts must
meet `cfg.ExperienceBulletsMin`.

**Rationale**: the clarified thresholds (spec FR-006) are vacancy-weighted, so a structural count
of skill *groups* cannot express them — a model can return all ten groups with the required skill
deleted from inside one. The existing `DropUngroundedSkillTokens` already tokenizes both sides, so
the same tokenizer is reused and the check stays a pure function of (master, merged, analysis).

**Alternatives considered**: group-count equality (simple, but blind to within-group deletion);
percentage of total content (one number to tune, ignores which content was lost).

## R5: Escalation mechanics

**Decision**: shortfall → retry once on the economy stage → second shortfall → re-run the selection
stage through the premium router, mark the run escalated.

**Rationale**: matches the clarified answer and reuses the existing `groundingAttempts` loop shape
in `tailorRendercvResume`, so escalation is a router swap on the final attempt rather than new
control flow. Bounded cost: escalation adds at most one premium selection call (~$0.04) and only
on double failure.

**Alternatives considered**: fail the run (cheapest, but costs the user the whole wait for a
transient economy hiccup); render marked-incomplete (ships the degraded document US2 exists to
prevent); deterministic fallback selection (no rephrasing, larger change).

## R6: Page-fit immutability

**Decision**: `expandContent` and `condenseContent` operate on selection content only. The summary
field is carried through untouched, and the merge step ignores any summary the page-fit stage
returns.

**Rationale**: clarified in session 2026-08-07 — a cheap model rewording the premium summary spends
the premium call for nothing, and re-verification would not catch it because a blander summary is
still perfectly grounded. Enforced structurally: the page-fit stages use `TailoredSelection`,
which has no summary field, so the failure mode is unrepresentable rather than merely forbidden.

**Alternatives considered**: route summary rewrites back to the premium stage (extra premium call
per refit); deterministic sentence-dropping (changes summary length semantics, unnecessary once
fitting works on selection alone).

## R7: The analysis → verifier coupling

**Decision**: when the vacancy analysis returns no required skills, the completeness verifier
falls back to a structural check (skill group count equals master's, highlights meet the
configured minimum) and records that it did.

**Rationale**: the clarified thresholds make stage 2's verifier depend on stage 1's output — a
sparse or empty analysis would make "every required skill retained" vacuously true, silently
disabling the gate this feature's second P1 story exists to provide. The fallback keeps a floor
under the check and the recorded reason makes the degradation visible rather than invisible.

**Alternatives considered**: failing the run on a thin analysis (a short vacancy posting is normal,
not an error); trusting the vacuous pass (reintroduces exactly the silent-truncation risk).

## R8: Cost and provenance recording

**Decision**: capture `usage.cost` and the served model from the gateway response per stage,
persist stage provenance on `GeneratedDocument`, and record per-stage timings and cost as activity
metadata.

**Rationale**: FR-017/SC-009 require measured rather than estimated economics. The gateway already
returns `usage.cost` (confirmed in every probe response) and the adapter currently discards it.
Without this, no cost claim in this feature can be verified after the fact — the exact cost of the
three shortlist runs already could not be reconstructed, because OpenRouter's `/activity` endpoint
rejects a normal inference key and only an account-lifetime total was available.

**Alternatives considered**: external billing integration (out of scope, needs a management key);
estimating from token counts (what was done during evaluation, ±40%).

## R9: Application-side deadlines

**Decision**: per-stage context deadlines in Go — analyze 90s, select 240s, summary 120s — rather
than relying on the proxy's `request_timeout`.

**Rationale**: `request_timeout: 60` was observed **not** to be enforced: one tailoring call hung
830 seconds until the application's own 14-minute timeout fired, and the fallback chain never
advanced. The proxy's timeout cannot be treated as a bound. Per-stage deadlines also cap the whole
run below the existing 14-minute handler timeout with room for retries.

**Alternatives considered**: trusting `request_timeout` (measured false); a single run-level
deadline only (a hung early stage consumes the entire budget, leaving nothing for the stages that
would have succeeded).
