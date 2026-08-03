# Contract: api health endpoints

## `GET /api/health` (existing, unchanged)

Liveness. Always `200 OK` while the process is running and accepting connections.

**Response** `200`:
```json
{ "ok": true, "uptime": 1234.5 }
```

## `GET /api/health/ready` (new)

Readiness. Checks Postgres, Redis, and MinIO (if configured) with a 2s per-dependency timeout.

**Response** `200` — all checked dependencies ok:
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

**Response** `503` — one or more dependencies unreachable:
```json
{
  "ok": false,
  "checks": {
    "postgres": { "status": "error", "error": "...", "latency_ms": 2001 },
    "redis":    { "status": "ok", "latency_ms": 1 },
    "minio":    { "status": "ok", "latency_ms": 8 }
  }
}
```

`minio` reports `{"status":"disabled"}` (no `error`/`latency_ms`) when `MINIO_ENDPOINT` is unset — this does not affect overall `ok`.

Consumers: `docker-compose.prod.yml`'s `api` healthcheck (`curl -f`), and any operator/on-call tooling that wants a machine-readable per-dependency breakdown.

## Compose healthcheck contracts (non-HTTP, informational)

| Service | `test` command | Meaning of success |
|---|---|---|
| `api` | `curl -f http://localhost:<PORT>/api/health/ready` | curl exit 0 only on HTTP 2xx — i.e. readiness `ok:true` |
| `ollama` | `ollama list` | CLI exit 0 only if it can reach the local API server |
| `minio` | `bash -c 'exec 3<>/dev/tcp/127.0.0.1/9000 && printf ... >&3 && read -r line <&3 && [[ "$line" == *200* ]]'` | first HTTP response line from `/minio/health/live` contains `200` |
| `postgres` (existing, unchanged) | `pg_isready -U jobfinder` | native Postgres readiness check |
