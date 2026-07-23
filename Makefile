.PHONY: install dev build typecheck up down logs ps prod-up prod-down prod-build clean \
	test test-go test-react test-python test-integration test-e2e test-lint test-db-setup \
	seed seed-clean truncate-db sqlc-generate sqlc-check sqlc-install \
	tygo-generate tygo-check tygo-install

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

# --- dev infra (postgres/redis/jobspy-sidecar) ---
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
test: test-go test-react test-python

test-go:
	cd apps/api && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		go test ./...

test-react:
	cd apps/dashboard && npx vitest run

test-python:
	cd apps/jobspy-sidecar && pytest

test-integration: test-db-setup
	@echo "Waiting for postgres to be healthy..."
	@docker compose up -d postgres
	@docker compose exec -T postgres sh -c 'until pg_isready -U jobfinder; do sleep 1; done'
	cd apps/api && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		go test -tags integration ./...

test-e2e: test-db-setup
	@echo "Waiting for services to be ready..."
	@docker compose up -d
	@sleep 5
	cd apps/dashboard && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		npx playwright test

test-lint: test-go test-react test-python

# --- Go API server ---
run-backend:
	cd apps/api && go run ./cmd/server

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

# --- go seed data ---
seed:
	cd apps/api && go run ./cmd/seed

seed-clean:
	cd apps/api && go run ./cmd/seed -clean

truncate-db:
	docker compose exec -T postgres psql -U jobfinder -d jobfinder -c \
		'TRUNCATE TABLE "Application","GeneratedDocument","Job","JobSource","MatchResult","Profile","SalaryCache","SavedSearch","SourceRun","Subscription" RESTART IDENTITY CASCADE;'
