# Data Model: Health/Readiness Endpoints + Compose Healthchecks

No persistent storage entities — this feature is entirely runtime health signals, not domain data. Documented here for shape/contract clarity only.

## ReadinessReport (response body of `GET /api/health/ready`)

| Field | Type | Values | Notes |
|---|---|---|---|
| `ok` | bool | `true`/`false` | `true` only if every non-disabled dependency check succeeded |
| `checks` | object (map) | keyed by dependency name | `postgres`, `redis`, `minio` |
| `checks.<dep>.status` | string | `"ok"` \| `"error"` \| `"disabled"` | `"disabled"` only ever applies to `minio` when `MINIO_ENDPOINT` is unset |
| `checks.<dep>.error` | string, omitted if status=`ok`/`disabled` | free text | underlying error message, truncated/sanitized (no credentials) |
| `checks.<dep>.latency_ms` | number, omitted if status=`disabled` | ≥0 | time taken for that dependency's ping |

Example (all healthy):
```json
{
  "ok": true,
  "checks": {
    "postgres": { "status": "ok", "latency_ms": 3 },
    "redis":    { "status": "ok", "latency_ms": 1 },
    "minio":    { "status": "ok", "latency_ms": 8 }
  }
}
```

Example (Postgres down, MinIO disabled):
```json
{
  "ok": false,
  "checks": {
    "postgres": { "status": "error", "error": "dial tcp 10.0.0.5:5432: connect: connection refused", "latency_ms": 2001 },
    "redis":    { "status": "ok", "latency_ms": 1 },
    "minio":    { "status": "disabled" }
  }
}
```

## State transitions (readiness handler, per request — stateless)

```
request in
  → for each configured dependency (postgres, redis, minio-if-enabled):
      run ping with bounded timeout (2s, per FR-004)
      record status ok/error + latency
  → ok = AND of all non-disabled checks' status == "ok"
  → HTTP 200 if ok else HTTP 503
  → JSON body = ReadinessReport
```

No state is stored between requests; each call re-checks live connectivity.

## LivenessReport (existing, unchanged — `GET /api/health`)

```json
{ "ok": true, "uptime": 1234.5 }
```
Unchanged from current `apps/api/internal/httpapi/router.go:91` — documented here only to make the liveness/readiness contrast explicit.
