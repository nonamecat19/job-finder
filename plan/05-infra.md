# 05 — Infrastructure (docker compose)

## Services

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment: { POSTGRES_DB: jobfinder, POSTGRES_USER: jobfinder, POSTGRES_PASSWORD: ${DB_PASSWORD} }
    volumes: [pgdata:/var/lib/postgresql/data]

  redis:
    image: redis:7-alpine          # BullMQ backend

  ollama:
    image: ollama/ollama:latest
    volumes: [ollama:/root/.ollama]
    # GPU (NVIDIA): uncomment
    # deploy: { resources: { reservations: { devices: [{ driver: nvidia, count: all, capabilities: [gpu] }] } } }

  ollama-init:                     # one-shot: pulls models on first up
    image: ollama/ollama:latest
    depends_on: [ollama]
    entrypoint: >
      sh -c "sleep 5 &&
             OLLAMA_HOST=http://ollama:11434 ollama pull ${LLM_MODEL} &&
             OLLAMA_HOST=http://ollama:11434 ollama pull nomic-embed-text"
    restart: "no"

  api:
    build: { context: ., dockerfile: apps/api/Dockerfile }   # includes chromium for Playwright/Puppeteer
    env_file: .env
    depends_on: [postgres, redis, ollama]
    volumes: [documents:/data/documents]
    ports: ["3000:3000"]           # dev only; nginx proxies in normal use

  dashboard:
    build: { context: ., dockerfile: apps/dashboard/Dockerfile }  # vite build → nginx
    ports: ["8080:80"]             # nginx: static + proxy /api → api:3000

  jobspy-sidecar:
    build: apps/jobspy-sidecar     # python:3.12-slim + python-jobspy + fastapi
    # internal only, no host port

  flaresolverr:                    # optional, profile-gated
    image: ghcr.io/flaresolverr/flaresolverr:latest
    profiles: [scraping-extras]

volumes: { pgdata: {}, ollama: {}, documents: {} }
```

`docker compose --profile scraping-extras up` enables FlareSolverr.

## Ollama model choice

| Model | VRAM/RAM | Role |
|---|---|---|
| `qwen2.5:14b` | ~10 GB | Default `LLM_MODEL` — best local quality for structured extraction + generation |
| `llama3.1:8b` | ~6 GB | Fallback on weaker hardware |
| `nomic-embed-text` | tiny | Embeddings (768 dims) |

CPU-only works but generation takes minutes; GPU strongly recommended. Model is env-switchable without code change.

## .env layout (`.env.example`)

```
DB_PASSWORD=...
DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@postgres:5432/jobfinder
REDIS_URL=redis://redis:6379

LLM_PROVIDER=ollama
OLLAMA_URL=http://ollama:11434
LLM_MODEL=qwen2.5:14b
EMBED_MODEL=nomic-embed-text

CONFIG_ENCRYPTION_KEY=...        # aes-256-gcm for JobSource.config

ADZUNA_APP_ID=... ADZUNA_APP_KEY=...
DJINNI_SESSION_COOKIE=...        # optional
JOBSPY_URL=http://jobspy-sidecar:8000
FLARESOLVERR_URL=                # optional
```

## Dev workflow

- `docker compose up postgres redis ollama` + `pnpm dev` (api and dashboard on host) for fast iteration.
- Full stack: `docker compose up --build`.
- Prisma migrations run on api startup (`prisma migrate deploy`).
- Dashboard at `http://localhost:8080`. LAN exposure only; if ever exposed beyond LAN, add basic auth at nginx — the app has no auth in v1.
