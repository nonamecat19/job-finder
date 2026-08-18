# Contract: Interactive HTTP Surface

**Feature**: 047-langchain-ai-service | Binding on `apps/ai` (FastAPI) and
`apps/api/internal/aiclient`.

Queued work travels as events (contracts/messaging.md). Capabilities invoked while a user waits
travel over HTTP, because a broker round-trip for a synchronous request buys nothing and costs
latency (research R9). Same service, same registry, same traces.

## H1. Scope

- **H1-1**: The HTTP surface serves exactly the interactive capabilities marked `HTTP` in
  contracts/capabilities.md C2: `rephrase` (called by both the keyword path and coach chat),
  `recruiter`, `outreach` and `embed`. Interview prep and company intel are **not** on this
  list — neither makes a model call.
- **H1-2**: A queued work type MUST NOT be served over HTTP (FR-027). The registry MUST enforce
  this: a capability whose transport is `event` MUST NOT be reachable through `/invoke`, and a
  request naming one MUST return 404 as though it did not exist.
- **H1-3**: The service MUST be reachable only from inside the stack. It MUST NOT be exposed
  through the gateway, the dashboard, or a published port in production.

## H2. Endpoints

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/capabilities/{name}/invoke` | Run an interactive capability |
| `GET` | `/health/live` | Process is up |
| `GET` | `/health/ready` | Registry valid, broker connected, gateway configured |

- **H2-1**: One generic invoke endpoint, not one per capability — capabilities are registry
  entries, and adding one MUST NOT require an endpoint (US2).
- **H2-2**: An unknown `{name}` MUST return 404 naming the capability. There is no default
  capability, carrying forward the fail-loudly rule of 044.

## H3. Request

```json
{
  "input": { "...": "the capability's declared input model" },
  "context": {
    "user_id": "usr_01H...",
    "work_id": "job_01H...",
    "activity_id": null
  }
}
```

- **H3-1**: `input` MUST validate against the capability's input model; a mismatch is 422 with
  the offending field named, never a best-effort run.
- **H3-2**: `context.user_id` and `context.work_id` are required — they are what make the trace
  findable (FR-014).
- **H3-3**: The request MUST carry every piece of data the capability needs. The service reads
  no database (FR-008).
- **H3-4**: A request MUST carry a client-generated request id, echoed in the response and used
  as the trace's correlation metadata.

## H4. Response

```json
{
  "status": "succeeded",
  "result": { "...": "the capability's declared output model" },
  "failure": null,
  "trace_id": "018f...",
  "usage": { "input_tokens": 812, "output_tokens": 96, "cost_usd": 0.0004 }
}
```

- **H4-1**: The body mirrors the result event shape (contracts/events.md E4), so both transports
  produce one result type.
- **H4-2**: HTTP status: 200 on `succeeded`; 4xx on `invalid_input`; 429 on `rate_limited` with
  `Retry-After`; 503 on `provider_unavailable` / `model_unavailable`; 504 on `timeout`; 500 on
  `internal` / `bound_exceeded`.
- **H4-3**: `trace_id` MUST be returned on both success and failure, so an operator can jump
  from a failed user request straight to its trace.
- **H4-4**: A failure body MUST NOT contain profile or posting content (E4-3).

## H5. Streaming

- **H5-1**: No capability streams today. Coach chat — the one candidate — makes a single
  `rephrase` call and returns a whole response, so `/stream` is **not** part of this feature.
  An earlier draft specified it on the mistaken belief that coach ran a tool loop.
- **H5-2**: If streaming is wanted later it is a separate feature, and these rules apply when
  it arrives: a stream MUST terminate with an explicit completion or failure event (a closed
  connection is a failure, not an implicit success); bounds apply identically (C4); and every
  step MUST still produce spans (C5-3) — streaming changes the transport, not the trace.

## H6. Client behaviour (Go side)

- **H6-1**: `internal/aiclient` MUST carry a per-capability timeout exceeding the capability's
  whole-run timeout (C4-4).
- **H6-2**: The client MUST NOT retry a request that failed with a non-retryable category
  (E5-2). Retrying a `rate_limited` response MUST honour `Retry-After`.
- **H6-3**: The client MUST surface `trace_id` into the backend's structured logs for the
  request, so a user-facing error is traceable without searching Langfuse by hand.
- **H6-4**: An unreachable AI service MUST produce a clear user-facing error. There is no
  fallback to a Go implementation once a capability's Go path is deleted (FR-023, C8-4), and no
  substituted result (FR-004 spirit).

## H7. Authentication

- **H7-1**: Requests MUST carry a shared secret configured for the stack; an unauthenticated
  request is 401.
- **H7-2**: The AI service MUST NOT accept end-user credentials or session tokens. Authorization
  happens in the backend before the call; the service trusts its caller and is not reachable by
  anyone else (H1-3).

## H8. Health and readiness

- **H8-1**: `/health/ready` MUST verify registry validity, broker connection and gateway
  *configuration*.
- **H8-2**: Readiness MUST NOT issue a model request as a probe — carrying forward 044-K1-3, so
  health checks never spend tokens.
- **H8-3**: An unavailable Langfuse collector MUST NOT make the service unready (FR-016).
