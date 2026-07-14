.PHONY: install dev build typecheck up down logs ps prod-up prod-down prod-build clean \
	test test-go test-react test-python test-integration test-e2e test-lint \
	seed seed-clean truncate-db

install:
	pnpm install

dev:
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

# --- tests ---
test: test-go test-react test-python

test-go:
	cd apps/api && go test ./...

test-react:
	cd apps/dashboard && npx vitest run

test-python:
	cd apps/jobspy-sidecar && pytest

test-integration: up
	@echo "Waiting for postgres to be healthy..."
	@docker compose up -d postgres
	@docker compose exec -T postgres sh -c 'until pg_isready -U jobfinder; do sleep 1; done'
	cd apps/api && go test -tags integration ./...

test-e2e: up
	@echo "Waiting for services to be ready..."
	@docker compose up -d
	@sleep 5
	cd apps/dashboard && npx playwright test

test-lint: test-go test-react test-python

# --- go seed data ---
seed:
	cd apps/api && go run ./cmd/seed

seed-clean:
	cd apps/api && go run ./cmd/seed -clean

truncate-db:
	docker compose exec -T postgres psql -U jobfinder -d jobfinder -c \
		'TRUNCATE TABLE "Application","GeneratedDocument","Job","JobSource","MatchResult","Profile","SavedSearch","SourceRun","Subscription" RESTART IDENTITY CASCADE;'
