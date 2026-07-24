# Quickstart: Validate Health/Readiness Endpoints + Compose Healthchecks

## Prerequisites

- Docker + Docker Compose v2
- `.env` populated per `.env.example` (needs `DB_PASSWORD` at minimum)

## 1. Bring up the stack

```sh
docker compose up -d
```

## 2. Confirm every service reports a health status (SC-001, SC-003)

```sh
docker compose ps
```

Expect `postgres`, `ollama`, `minio` (and `api` in prod compose) to show `healthy` (or `starting` briefly right after boot) — not just `Up`/`running` with no health column.

## 3. Exercise the api readiness endpoint directly (User Story 2)

```sh
curl -s http://localhost:3000/api/health/ready | jq
curl -s http://localhost:3000/api/health | jq   # liveness, unaffected by dependency state
```

Expect `ready` to return `{"ok": true, "checks": {...}}` with `200`.

## 4. Break a dependency and observe readiness flip (SC-002)

```sh
docker compose stop postgres
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:3000/api/health/ready   # expect 503
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:3000/api/health         # expect 200 (still alive)
docker compose start postgres
```

Wait one healthcheck interval (~5s) and re-run `docker compose ps` — `api`'s compose health status should flip back to `healthy` (SC-004) without restarting the container.

## 5. Probe ollama and minio health signals directly (User Story 3)

```sh
docker compose exec ollama ollama list        # exit 0 = healthy
docker compose exec minio bash -c '
  exec 3<>/dev/tcp/127.0.0.1/9000 &&
  printf "GET /minio/health/live HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n" >&3 &&
  read -r line <&3 &&
  echo "$line"'
```

Expect the `ollama list` command to exit 0, and the minio probe to print a line containing `200`.

## Related contracts / data shapes

- Endpoint request/response shapes: [contracts/health-api.md](./contracts/health-api.md)
- Response field definitions: [data-model.md](./data-model.md)
