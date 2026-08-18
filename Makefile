.PHONY: install dev build typecheck up down logs ps prod-up prod-down prod-build clean run-all \
	test test-go test-react test-extension test-py test-integration test-e2e test-ai-optional test-lint test-db-setup \
	truncate-db sqlc-generate sqlc-check sqlc-install \
	tygo-generate tygo-check tygo-install \
	contracts-generate contracts-check \
	lint lint-go lint-web lint-py golangci-install \
	audit vuln-go vuln-web vuln-py check-ai-no-provider-sdk check-ai-env secrets images

ifneq (,$(wildcard .env))
include .env
export
endif

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

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

prod-build:
	docker compose -f docker-compose.prod.yml build

prod-up:
	docker compose -f docker-compose.prod.yml up -d

prod-down:
	docker compose -f docker-compose.prod.yml down

clean:
	docker compose down -v
	rm -rf node_modules apps/*/node_modules apps/*/dist packages/*/node_modules packages/*/dist

test-db-setup: up
	@docker compose exec -T postgres psql -U jobfinder -d postgres -c "DROP DATABASE IF EXISTS jobfinder_test;" 2>/dev/null || true
	@docker compose exec -T postgres createdb -U jobfinder jobfinder_test

test: test-go test-react test-extension

test-go:
	cd apps/api && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		go test ./...

test-react:
	cd apps/dashboard && pnpm exec vitest run

test-extension:
	cd apps/extension && pnpm exec vitest run

test-py:
	cd apps/ai && uv run pytest

# Needs a Docker daemon and nothing else: every backing service the integration
# suite touches (Postgres, RabbitMQ, ClickHouse, MinIO, Redis, headless Chrome,
# FlareSolverr, and the LiteLLM proxy on the real gateway/config.yaml) is
# started as a throwaway container by apps/api/internal/testinfra. The suite
# never borrows the dev stack's data and never silently skips a service that
# happened not to be running; no provider credential is involved, since the
# proxy's upstreams point at an in-test stub.
test-integration:
	cd apps/api && go test -tags integration ./...

test-ai-optional:
	./scripts/test-ai-optional.sh

test-e2e: test-db-setup
	@echo "Waiting for services to be ready..."
	@docker compose up -d
	@sleep 5
	cd apps/dashboard && DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:${POSTGRES_HOST_PORT}/jobfinder_test \
		REDIS_URL=redis://localhost:6379/1 \
		npx playwright test

lint-go:
	./scripts/golangci-check.sh

lint-web:
	pnpm exec eslint apps/dashboard apps/extension packages/shared

lint-py:
	cd apps/ai && uv run ruff check . && uv run ruff format --check . && uv run mypy src

lint: lint-go lint-web lint-py

test-lint: lint-go lint-web lint-py test-go test-react test-extension test-py

vuln-go:
	./scripts/govulncheck-check.sh

vuln-web:
	pnpm audit --audit-level=high --prod=false

vuln-py:
	cd apps/ai && uv run pip-audit

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

images:
	docker build -f apps/api/Dockerfile       -t job-finder-api:local-check       .
	docker build -f apps/dashboard/Dockerfile -t job-finder-dashboard:local-check .

audit: vuln-go vuln-web vuln-py check-ai-no-provider-sdk check-ai-env secrets

run-all: up
	@echo "Starting backend and frontend..."
	$(MAKE) run-backend &
	$(MAKE) run-frontend

run-backend:
	cd apps/api && go run ./cmd/server

run-backend-hot:
	cd apps/api && air -c .air.toml

SQLC_VERSION := $(shell tr -d '[:space:]' < apps/api/.sqlc-version)

sqlc-install:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v$(SQLC_VERSION)

sqlc-generate:
	cd apps/api && sqlc generate

sqlc-check:
	./scripts/sqlc-check.sh

TYGO_VERSION := $(shell tr -d '[:space:]' < apps/api/.tygo-version)

tygo-install:
	go install github.com/gzuidhof/tygo@v$(TYGO_VERSION)

tygo-generate:
	cd apps/api && tygo generate

tygo-check:
	./scripts/tygo-check.sh

contracts-generate:
	cd apps/api && go run ./cmd/contractsgen
	./scripts/contracts-generate-ai.sh

contracts-check:
	./scripts/contracts-check.sh

truncate-db:
	docker compose exec -T postgres psql -U jobfinder -d jobfinder -c \
		'TRUNCATE TABLE "Application","GeneratedDocument","Job","JobSource","MatchResult","Profile","SalaryCache","SavedSearch","SourceRun","Subscription" RESTART IDENTITY CASCADE;'
