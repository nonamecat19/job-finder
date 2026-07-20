# Phase 0 Research: Salary Inference

Decisions taken before design, with the alternatives that were rejected and why.

## Decision 1 — Reference dataset: the levels.fyi premise does not survive checking

**Task premise**: "levels.fyi PUBLIC CSV (downloaded at startup), CSV ToS is gray-area."

**Finding**: the premise conflates two different things, and neither is a public CSV in the sense the premise assumes.

| Path | Status |
|---|---|
| An official levels.fyi CSV export under an open licence | **Not found.** levels.fyi surfaces data through its site and paid products. No open-data export or public licence grant is evident. |
| Scraping levels.fyi at startup | **Not gray — against their terms.** Their terms prohibit automated collection. |
| Third-party redistributed scrapes (dataset-sharing sites) | **Genuinely gray.** These exist and are widely used. The licence attached is the redistributor's, applied to data they scraped; whether it is theirs to grant is the unresolved part. |

**Precedent within this repo**: [spec 001](../001-workua-robotaua-adapters/research.md) parked robota.ua rather than circumvent Cloudflare bot protection, reasoning that reaching the data meant working around the operator's stated position. Scraping levels.fyi is the same shape of decision. Deciding it differently here would need an explicit reason, and this document does not have one to offer.

**Rejected — scrape levels.fyi directly at startup**: against their terms, and inconsistent with the robota.ua precedent.

**Rejected — build against a redistributed scrape without checking its licence**: the cheapest path and the one the task implies, but it makes the project's compliance depend on a third party's unexamined claim.

**Decision**: treat `levels-fyi` as a **placeholder source name** and select an explicitly-licensed compensation dataset before writing the loader. Candidates to evaluate on licence first, coverage second:

- Government/statistical sources (e.g. national labour statistics) — unambiguous licence, coarse titles, weak non-US coverage.
- Stack Overflow's annual developer survey — openly licensed, self-reported, developer-specific, has real non-US coverage including UA.
- EU/OECD wage statistics — clean licence, very coarse buckets.

Coverage matters less than licence here because the dataset source is one of three and is designed to degrade honestly (Decision 5). A well-licensed dataset with mediocre coverage is strictly better than a well-covered one the project cannot defend using.

**Consequence for design**: the `"SalaryCache"` schema is dataset-agnostic (`titleBucket`, `geoBucket`, `companySizeBucket`, `source`) precisely so this decision can land late without a schema change. That was deliberate, not incidental.

## Decision 2 — Parse `salaryRaw` first, before anything else is built

The corpus already holds postings with stated compensation in `salaryRaw` (a free-text field populated by every adapter). Parsing it yields two things at once:

1. The `ingested-cache` source's entire input — the only source grounded in this user's actual job market rather than someone else's dataset.
2. A **labelled held-out set**. Hide the parsed value, estimate the job as if compensation were absent, compare. This is the only way SC-004 is measurable at all, and it needs no external ground truth.

That second use is why parsing is the first implementation task rather than merely the easiest one. Without it, accuracy claims about the LLM and dataset sources are unfalsifiable.

**Approach**: regex-based, table-driven, one case per pattern actually observed in the corpus. Not a general-purpose money grammar — the input is a bounded set of board conventions, and each new board adds a row to a test table.

Patterns the corpus requires handling: `$3000-5000`, `3000–5000 $` (en-dash, trailing symbol), `від 60000 грн`, `up to €70k`, `80k-100k PLN`, bare `$4500`, and non-numeric refusals (`competitive`, `договірна`, `negotiable`) which must parse to *nothing* rather than to zero.

**Rejected — LLM-parse `salaryRaw`**: a model call per job to read a string a regex handles deterministically. Slower, non-reproducible, and it would make the held-out set depend on the same component being evaluated — circular.

**Two traps, both silent:**

- **Currency symbols are ambiguous.** `$` is USD, CAD, and AUD. Store ISO 4217 codes, resolved using the posting's geo when the symbol alone is ambiguous.
- **Period is usually implicit.** A Ukrainian posting quoting `60000 грн` means per month. Treating it as annual is a 12× error that no downstream check would catch. `Period` is therefore explicit on `SalaryBand` and normalized before storage.

## Decision 3 — Confidence-weighted averaging, capped

The user specified it: weights are the confidences, normalized; the final confidence is their sum.

**The cap is an addition, and it is necessary.** Uncapped, three sources at 0.5 sum to 1.5 — not a probability, and it breaks the `< 0.3` threshold comparison and any downstream arithmetic that assumes a 0–1 range. Capped at 1.

**Invariant worth asserting**: a blend of exactly one source at confidence *c* must produce confidence *c*, not 1.0. A normalization that divides by the weight sum and then reports 1.0 for the single-source case is an easy bug and an invisible one — it would mark every single-source estimate as maximally trustworthy.

**Rejected — take the highest-confidence source outright**: simpler and more interpretable, but discards real information when two sources agree. Agreement between independent sources is evidence, and the summed confidence is what captures it.

**Rejected — intersect the intervals**: attractive because agreement narrows the band naturally. Fails when sources disagree entirely: the intersection is empty and there is no principled fallback. Disagreement is common enough here (a US-derived dataset vs. a model reading a Kyiv posting) that the degenerate case would be the normal case.

**Rejected — learn the weights from held-out data**: correct in principle and premature. It needs a labelled corpus that only exists after Decision 2's parser has run over a substantial ingest. Revisit once there is one.

## Decision 4 — Ground truth replaces, never blends

When `salaryRaw` parses, that band wins outright and `"salarySource"` records `posting`.

Blending a known figure with a guess produces something strictly worse than the known figure. The only argument for blending would be distrusting the parser, and the answer to a distrusted parser is to fix it, not to average around it.

This is also why `posting` exists as a fifth `"salarySource"` value beyond the four the task named. Collapsing it into `ingested-cache` would conflate "this posting states X" with "postings like this one pay about X" — precisely the distinction Story 3 exists to show the user, and the distinction that separates SC-003 (parser correctness) from SC-004 (estimate accuracy) as separate measurements.

## Decision 5 — Bucket normalization is where accuracy is actually won

The dataset source's real problem is not lookup, it is matching `Sr. Backend Engineer / Kyiv / unknown-size` against buckets built from a different labour market. This dominates any refinement to the blending arithmetic.

**Title**: lowercase, strip seniority prefixes into a separate signal, canonicalize synonyms (`software engineer` / `developer` / `programmer` → one bucket). Seniority is stripped from the title and reapplied as a multiplier, because otherwise every seniority level of every role needs its own populated bucket and sparsity gets multiplied.

**Geo**: country-level. City-level is too sparse to populate from any available source. `"*"` catch-all for postings with no geo.

**Company size**: `startup` / `mid` / `large` / `unknown`, inferred from posting hints. **`unknown` will be the common case** — postings rarely state headcount. Treating this dimension as reliable would be a mistake; it is the weakest of the three and the first to be widened away.

**Fallback widening**, in order: exact → company size to `unknown` → geo to `*`. Each widening lowers the source's reported confidence.

**This is why SC-004 pairs accuracy with confidence correlation.** For UA/EU roles the buckets may be near-empty. The correct behaviour is not a fabricated band — it is a wide band with an honest low confidence, which then blends weakly and is marked low-confidence in the UI. A source that fails loudly and correctly is more valuable than one that guesses quietly, and confidence calibration is the mechanism that makes that distinction visible.

## Decision 6 — Startup load, cached, non-blocking

Retrieve the dataset once at startup, upsert into `"SalaryCache"` keyed by the natural bucket key, serve every subsequent lookup from Postgres.

**Upsert, not append** — without the unique constraint on `(titleBucket, geoBucket, companySizeBucket, source)` the table grows by a full dataset on every restart, and lookups start returning duplicates.

**Non-blocking** — a failed retrieval logs and continues. Yesterday's cached copy remains valid; compensation data ages on a scale of months, not hours. SC-010 requires this, and it is also why the cache is a table rather than an in-memory map: an unreachable dataset must leave the previous copy on disk.

`"refreshedAt"` supports discounting stale buckets. Not used in v1's confidence calculation, but the column exists so that adding it later is not a migration.

**Rejected — fetch per job**: re-retrieving a static dataset per lookup. No.

**Rejected — vendor the dataset into the repo**: reproducible and offline, but bakes the unresolved licence question (Decision 1) directly into version control, which is the worst place for it.

## Open question — Story 3's breakdown has nowhere to live

`BlendedEstimate.Components` holds each source's independent band and confidence. That is exactly what FR-021 requires the detail page to show, and there is currently no place to persist it: the user ruled out a related table, and the alternative is a JSON column on `"Job"`.

Options:

1. **Recompute on request** — no schema change, but a model call per detail-page view. Almost certainly unacceptable on latency and cost.
2. **One JSON column on `"Job"`** — cheap, honest, and a small deviation from "five columns" rather than from any stated principle. The likely right answer.
3. **Store only the source names** — the current `"salarySource"` already does a weak version of this. It satisfies "which sources contributed" but not "what each one predicted", so it under-delivers FR-021 and SC-008.

**Unresolved. Blocks Story 3 only** — Stories 1 and 2 are unaffected and can ship first. Flagged in [plan.md](./plan.md) for a decision before Story 3 is built.
