# Build + run the NestJS API. Context = repo root (pnpm workspace).
FROM node:22-bookworm AS build

RUN corepack enable
WORKDIR /repo

COPY pnpm-workspace.yaml package.json ./
COPY packages/shared/package.json packages/shared/
COPY apps/api/package.json apps/api/
RUN pnpm install --no-frozen-lockfile --filter @job-finder/shared --filter @job-finder/api

COPY tsconfig.base.json ./
COPY packages/shared packages/shared
COPY apps/api apps/api

RUN pnpm --filter @job-finder/shared build \
 && cd apps/api && pnpm build

# ---------------------------------------------------------------------------
FROM node:22-bookworm

RUN corepack enable
WORKDIR /repo

COPY --from=build /repo /repo

# chromium + system deps for Playwright (scraping + PDF render)
RUN cd apps/api && npx playwright install --with-deps chromium \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /repo/apps/api
ENV NODE_ENV=production
EXPOSE 3000

# migrations on startup, then serve
CMD ["sh", "-c", "npx drizzle-kit migrate && node dist/main.js"]
