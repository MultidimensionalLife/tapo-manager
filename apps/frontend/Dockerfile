# Build context is the monorepo root (tapo-manager/), since this is a
# pnpm workspace package that needs the root lockfile/workspace manifest.

# --- build stage ---
FROM node:22-alpine AS build
WORKDIR /repo
RUN corepack enable

COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/frontend/package.json apps/frontend/package.json
RUN pnpm install --frozen-lockfile --filter tapo-manager-frontend...

COPY apps/frontend apps/frontend
RUN pnpm --filter tapo-manager-frontend build

# --- runtime stage ---
FROM nginx:1.27-alpine AS runtime
COPY apps/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/frontend/dist /usr/share/nginx/html
EXPOSE 80
