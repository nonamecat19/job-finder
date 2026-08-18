.PHONY: install dev build typecheck up down logs ps prod-up prod-down prod-build clean run-all \
	test test-go test-react test-py test-integration test-e2e test-ai-optional test-lint test-db-setup \
	truncate-db sqlc-generate sqlc-check sqlc-install \
	tygo-generate tygo-check tygo-install \
	contracts-generate contracts-check \
	lint lint-go lint-web lint-py golangci-install \
	audit vuln-go vuln-web vuln-py check-ai-no-provider-sdk check-ai-env secrets images

ifneq (,$(wildcard .env))
include .env
export
endif

# --- per-worktree isolation ---
# Each git worktree gets its own compose project and Postgres host port so
# migration state from one branch never leaks into another's test run.
WORKTREE_NAME := $(shell basename "$$(git rev-parse --show-toplevel 2>/dev/null || pwd)")
WORKTREE_HASH := $(shell echo "$(WORKTREE_NAME)" | cksum | cut -d' ' -f1)
export COMPOSE_PROJECT_NAME := jobfinder-$(WORKTREE_NAME)
export POSTGRES_HOST_PORT := $(shell echo "$$(( 5432 + ( $(WORKTREE_HASH) % 100 ) ))")

install:
	pnpm install

run-frontend:
	pnpm dev

build:
	pnpm build

typecheck:
	pnpm typecheck

# --- dev infra (postgres/redis) ---
up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

# --- prod stack (full app in containers) ---
prod-build:
	docker compose -f docker-compose.prod.yml build

prod-up:
	docker compose -f docker-compose.prod.yml up -d

prod-down:
	docker compose -f docker-compose.prod.yml down

clean:
	docker compose down -v
	rm -rf node_modules apps/*/node_modules apps/*/dist packages/*/node_modules packages/*/dist

# --- test database setup ---
test-db-setup: up
	@docker compose exec -T postgres psql -U jobfinder -d postgres -c "DROP DATABASE IF EXISTS jobfinder_test;" 2>/dev/null || true
	@docker compose exec -T postgres createdb -U jobfinder jobfinder_test

# --- tests ---
test: test-go test-react

test-go:
	cd apps/api && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		go test ./...

test-react:
	cd apps/dashboard && pnpm exec vitest run

test-py:
	cd apps/ai && uv run pytest

test-integration: test-db-setup
	@echo "Waiting for postgres to be healthy..."
	@docker compose up -d postgres
	@docker compose exec -T postgres sh -c 'until pg_isready -U jobfinder; do sleep 1; done'
	cd apps/api && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		go test -tags integration ./...

test-ai-optional:
	./scripts/test-ai-optional.sh

test-e2e: test-db-setup
	@echo "Waiting for services to be ready..."
	@docker compose up -d
	@sleep 5
	cd apps/dashboard && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		npx playwright test

# --- lint (specs/domains/platform-operations.md) ---
# `make lint`/`test-lint` are the only sanctioned way to invoke either
# linter — CI and the Stop hook call these same targets, never the binaries
# directly, so local, automated and hook-triggered runs cannot drift apart.
lint-go:
	./scripts/golangci-check.sh

lint-web:
	pnpm exec eslint apps/dashboard packages/shared

lint-py:
	cd apps/ai && uv run ruff check . && uv run ruff format --check . && uv run mypy src

lint: lint-go lint-web lint-py

test-lint: lint-go lint-web lint-py test-go test-react test-py

# --- supply-chain gates (039, specs/domains/platform-operations.md) ---
# Deliberately NOT part of `test-lint`, and that is an exemption from the
# coverage invariant rather than an oversight. Two reasons:
#
#   1. These gates are not deterministic in time. An advisory published this
#      afternoon turns this morning's green run red with no code change, so
#      "local success predicts CI" — the property the invariant exists to
#      protect — was never available here.
#   2. They need the network, and `images` needs Docker: the same exemption
#      `test-integration` and `test-e2e` already hold.
#
# `make images` additionally costs 6-8 minutes cold, which would make the
# pre-push loop slower than CI and push people to skip it entirely.
vuln-go:
	./scripts/govulncheck-check.sh

vuln-web:
	pnpm audit --audit-level=high --prod=false

vuln-py:
	cd apps/ai && uv run pip-audit

# Architectural invariants for the AI service, not vulnerability scans, but
# gated alongside them: both are "the AI service must not be able to reach a
# provider directly" checks, just at different layers (dependency tree vs.
# runtime environment) (C7-3, K2-2, K2-3, FR-008, FR-011).
check-ai-no-provider-sdk:
	./scripts/check-ai-no-provider-sdk.sh

check-ai-env:
	./scripts/check-ai-service-env.sh

secrets:
	@command -v gitleaks >/dev/null 2>&1 || { \
		echo "error: gitleaks is not installed."; \
		echo ""; \
		echo "Install the pinned version:"; \
		echo ""; \
		echo "  VERSION=$$(tr -d '[:space:]' < .gitleaks-version); \\"; \
		echo "  curl -sSfL \"https://github.com/gitleaks/gitleaks/releases/download/v\$$VERSION/gitleaks_\$${VERSION}_linux_x64.tar.gz\" \\"; \
		echo "    | sudo tar -xz -C /usr/local/bin gitleaks"; \
		echo ""; \
		echo "The version is pinned so a local run and a CI run reach the same verdict."; \
		exit 1; \
	}
	gitleaks git . --redact --no-banner --config .gitleaks.toml

# Build only — never pushes, never needs a registry credential.
images:
	docker build -f apps/api/Dockerfile       -t job-finder-api:local-check       .
	docker build -f apps/dashboard/Dockerfile -t job-finder-dashboard:local-check .

# The audit-class gates as one entry point, first non-zero wins — the same
# shape as `make lint`. `images` stays out: it is the slow one.
audit: vuln-go vuln-web vuln-py check-ai-no-provider-sdk check-ai-env secrets

# --- run all (infra + backend + frontend) ---
run-all: up
	@echo "Starting backend and frontend..."
	$(MAKE) run-backend &
	$(MAKE) run-frontend

# --- Go API server ---
run-backend:
	cd apps/api && go run ./cmd/server

# Hot-reload backend via air (github.com/air-verse/air). Install: go install github.com/air-verse/air@latest
run-backend-hot:
	cd apps/api && air -c .air.toml

# --- sqlc code generation ---
# Version is pinned in apps/api/.sqlc-version so local and CI emit identical code.
SQLC_VERSION := $(shell tr -d '[:space:]' < apps/api/.sqlc-version)

sqlc-install:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v$(SQLC_VERSION)

sqlc-generate:
	cd apps/api && sqlc generate

# Fails if apps/api/internal/db/sqlcgen is stale. Mirrors the API CI job.
sqlc-check:
	./scripts/sqlc-check.sh

# --- tygo code generation (Go DTOs -> packages/shared/src/generated.ts) ---
# Version is pinned in apps/api/.tygo-version so local and CI emit identical code.
TYGO_VERSION := $(shell tr -d '[:space:]' < apps/api/.tygo-version)

tygo-install:
	go install github.com/gzuidhof/tygo@v$(TYGO_VERSION)

tygo-generate:
	cd apps/api && tygo generate

# Fails if packages/shared/src/generated.ts is stale. Mirrors the API CI job.
tygo-check:
	./scripts/tygo-check.sh

# --- contracts code generation (Go event structs -> JSON Schema -> apps/ai Pydantic models) ---
contracts-generate:
	cd apps/api && go run ./cmd/contractsgen
	./scripts/contracts-generate-ai.sh

# Fails if apps/api/internal/events/schema or apps/ai/src/jobfinder_ai/contracts
# is stale. Mirrors sqlc-check/tygo-check.
contracts-check:
	./scripts/contracts-check.sh

truncate-db:
	docker compose exec -T postgres psql -U jobfinder -d jobfinder -c \
		'TRUNCATE TABLE "Application","GeneratedDocument","Job","JobSource","MatchResult","Profile","SalaryCache","SavedSearch","SourceRun","Subscription" RESTART IDENTITY CASCADE;'
