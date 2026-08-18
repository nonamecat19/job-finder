# Build + run the AI orchestration service (047-langchain-ai-service).
# apps/ai is a uv-managed Python 3.13 project; apps/ai/pyproject.toml and
# apps/ai/uv.lock pin every dependency (langchain, langgraph, faststream,
# fastapi — see specs/047-langchain-ai-service/tasks.md T006). Context =
# repo root, matching apps/api/Dockerfile, so only apps/ai is copied in.
FROM python:3.13-slim AS build

RUN pip install --no-cache-dir uv==0.9.7

WORKDIR /src
COPY apps/ai/pyproject.toml apps/ai/uv.lock ./
RUN uv sync --frozen --no-install-project --no-dev

COPY apps/ai .
RUN uv sync --frozen --no-dev

# ---------------------------------------------------------------------------
# Runtime: the synced virtualenv plus the source tree, no build toolchain.
FROM python:3.13-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /src /src
ENV PATH="/src/.venv/bin:$PATH"
WORKDIR /src

EXPOSE 8000
# Runs the FastAPI app and the FastStream consumers in one process
# (research R9) — /health/live and /health/ready are served here.
CMD ["python", "-m", "jobfinder_ai.main"]
