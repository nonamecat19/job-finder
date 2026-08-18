# Contract: Event Envelope and Payloads

**Feature**: 047-langchain-ai-service | Source of truth: Go structs in
`apps/api/internal/events`, generated to JSON Schema, then to Python Pydantic models (research
R7). Neither generated artifact is hand-edited.

## E1. Envelope

Every message carries this envelope. Field semantics in data-model.md § 1.

```json
{
  "event_id": "018f...",
  "event_type": "match.requested",
  "schema_version": 1,
  "occurred_at": "2026-08-18T10:31:02Z",
  "work_id": "job_01H...",
  "correlation_id": "018f...",
  "idempotency_key": "match:job_01H...:1",
  "run_id": "018f...",
  "activity_id": "act_01H...",
  "trace_id": null
}
```

- **E1-1**: All fields except `activity_id` and `trace_id` are required on every message.
- **E1-2**: `trace_id` is null on work events and set on result events emitted by the
  orchestration service (FR-014).
- **E1-3**: `correlation_id` on a result MUST equal that of its work event.
- **E1-4**: The envelope MUST NOT contain profile, posting or resume content.

## E2. Event type registry

Enumerated and closed. An event type outside this list is dead-lettered (M6-2).

| `event_type` | Direction | Payload |
|---|---|---|
| `ingest.requested` | backend → Go consumer | `IngestPayload` |
| `ingest.completed` | Go consumer → backend | ingest result |
| `enrich.requested` | backend → Go consumer | `EnrichPayload` |
| `enrich.completed` | Go consumer → backend | enrich result |
| `match.requested` | backend → AI service | `MatchPayload` + snapshot |
| `match.completed` | AI service → backend | `MatchResult` |
| `generate.requested` | backend → AI service | `GeneratePayload` + snapshot |
| `generate.completed` | AI service → backend | `GenerationResult` |
| `salary.requested` | backend → AI service | `SalaryInferPayload` + snapshot |
| `salary.completed` | AI service → backend | `SalaryResult` |
| `ghost.requested` | backend → AI service | `GhostScorePayload` + snapshot |
| `ghost.completed` | AI service → backend | `GhostResult` |

- **E2-1**: `*.completed` carries `status` and is used for both success and failure — there is
  no separate `*.failed` type. Failure is a status, not an event type, so a consumer handles one
  shape (data-model.md § 3).

## E3. Work payloads

- **E3-1**: The existing `internal/queue` payload structs are the payload contract and MUST NOT
  be reshaped by this feature. Comparability of before/after behaviour depends on it.
- **E3-2**: AI work events additionally carry an **input snapshot** — the profile and posting
  data the capability needs — because FR-008 denies the orchestration service database access.
- **E3-3**: The snapshot MUST be the complete grounding source for the run. The orchestration
  service MUST NOT fetch supplementary content (Constitution II).
- **E3-4**: Every snapshot carries `snapshot_hash`, computed at publish, echoed on the result
  (data-model.md § 3).
- **E3-5**: A published message MUST NOT exceed a configured maximum size. Exceeding it is a
  publish error naming the work id and the size — never a silently truncated snapshot.

## E4. Result payloads

```json
{
  "status": "succeeded",
  "result": { "...": "capability-specific, see contracts/capabilities.md" },
  "failure": null,
  "trace_id": "018f...",
  "snapshot_hash": "sha256:...",
  "usage": { "input_tokens": 3120, "output_tokens": 412, "cost_usd": 0.0021 }
}
```

- **E4-1**: Exactly one of `result` / `failure` is non-null, determined by `status`.
- **E4-2**: `result` MUST validate against the capability's declared output schema. A result
  that does not validate MUST be published as a failure with category `internal`, never as a
  success (FR-003).
- **E4-3**: `failure.message` MUST NOT embed profile or posting content — it is written to
  application logs, which are not on the 30-day payload purge (FR-018).
- **E4-4**: `usage` is best-effort: absent when the gateway does not report it, never a reason
  to fail a run.

## E5. Failure categories

```
rate_limited | credential_rejected | insufficient_credits | model_unavailable |
provider_unavailable | invalid_input | bound_exceeded | timeout | internal
```

- **E5-1**: The first five map one-to-one onto the existing sentinels in
  `internal/platform/llm/infrastructure/shared/errors.go` and MUST keep those names.
- **E5-2**: `retryable` is set by the producer of the failure, not re-derived by the consumer
  (FR-004). Defaults: `rate_limited`, `provider_unavailable`, `timeout`, `internal` retryable;
  `credential_rejected`, `insufficient_credits`, `invalid_input`, `bound_exceeded` not.
- **E5-3**: `bound_exceeded` MUST name the bound in `failure.message` (step count, tool rounds,
  or timeout) so FR-005 violations are diagnosable without opening the trace.
- **E5-4**: `failed_step` MUST be set for any capability implemented as a graph (FR-040).

## E6. Versioning

- **E6-1**: `schema_version` starts at 1 for every event type and is versioned per type, not
  globally.
- **E6-2**: Compatible change (adding an optional field) → no bump. Breaking change (removal,
  rename, semantic change) → bump (M6-3).
- **E6-3**: During a version transition both consumers MUST be deployed before the publisher
  begins emitting the new version.

## E7. Generation

- **E7-1**: `make contracts-generate` regenerates JSON Schema from the Go structs and Pydantic
  models from the schemas.
- **E7-2**: `make contracts-check` regenerates into a temporary location and diffs; a
  difference fails CI. This mirrors `sqlc-check` and `tygo-check` (Constitution III).
- **E7-3**: Generated Python under `apps/ai/src/jobfinder_ai/contracts/` MUST carry a
  do-not-edit header and MUST NOT be modified by hand.
