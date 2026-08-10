# Build the Vite dashboard and serve via nginx (static + /api proxy).
FROM node:26-bookworm AS build

RUN corepack enable
WORKDIR /repo

COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY packages/shared/package.json packages/shared/
COPY apps/dashboard/package.json apps/dashboard/
# Frozen, and the lockfile is copied in so it can be. This used to install with
# --no-frozen-lockfile, which was load-bearing only because pnpm-lock.yaml was
# gitignored (039): with no lockfile to freeze against, the image resolved its
# own dependency graph at build time. That made the set the audit gate checks
# and the set the image ships two different sets, which is most of the value of
# having an audit gate.
RUN pnpm install --frozen-lockfile --filter @job-finder/shared --filter @job-finder/dashboard

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
