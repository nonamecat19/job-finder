# Build the Vite dashboard and serve via nginx (static + /api proxy).
FROM node:22-bookworm AS build

RUN corepack enable
WORKDIR /repo

COPY pnpm-workspace.yaml package.json ./
COPY packages/shared/package.json packages/shared/
COPY apps/dashboard/package.json apps/dashboard/
RUN pnpm install --no-frozen-lockfile --filter @job-finder/shared --filter @job-finder/dashboard

COPY tsconfig.base.json ./
COPY packages/shared packages/shared
COPY apps/dashboard apps/dashboard

RUN pnpm --filter @job-finder/shared build \
 && pnpm --filter @job-finder/dashboard build

# ---------------------------------------------------------------------------
FROM nginx:alpine

COPY apps/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/dashboard/dist /usr/share/nginx/html

EXPOSE 80
