.PHONY: install dev build typecheck up down logs ps prod-up prod-down prod-build \
        db-migrate db-generate db-studio clean

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

# --- drizzle (api) ---
db-generate:
	pnpm --filter @job-finder/api db:generate

db-migrate:
	pnpm --filter @job-finder/api db:migrate

db-studio:
	pnpm --filter @job-finder/api db:studio

clean:
	docker compose down -v
	rm -rf node_modules apps/*/node_modules apps/*/dist packages/*/node_modules packages/*/dist
