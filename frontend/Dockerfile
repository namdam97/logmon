# syntax=docker/dockerfile:1
# Multi-stage build cho Next.js (App Router/RSC) → standalone Node server, non-root.
# Khớp pattern prod backend: image nhỏ, không shell thừa, chạy user không phải root.

# ── deps: cài dependency với lockfile cố định ──────────────────────────────────
FROM node:22-alpine AS deps
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

# ── build: biên dịch Next standalone ───────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /app
RUN corepack enable
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

# ── runner: chỉ artifact standalone + static ──────────────────────────────────
FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0
# Non-root (node user có sẵn trong image, uid 1000).
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 3000
# Liveness/readiness do Nginx + compose healthcheck đảm nhiệm (Next không có /healthz).
CMD ["node", "server.js"]
