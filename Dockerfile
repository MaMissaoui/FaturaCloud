# syntax=docker/dockerfile:1

# ---- Stage 1: Build the React frontend ----
FROM node:22-alpine AS frontend
WORKDIR /app

RUN npm install -g pnpm

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY index.html lingui.config.ts vite.config.ts tsconfig.json tsconfig.node.json ./
COPY src ./src
COPY public ./public

# VERSION also feeds vite.config.ts's sentryVitePlugin release name, so uploaded
# source maps match the release string the running app reports to Sentry.
ARG VERSION=dev
ENV VERSION=$VERSION

# Optional: bake a Sentry DSN into this build so the app reports errors. Left
# empty by default — the published multi-arch image is built without one, so
# Sentry stays off unless a deployment explicitly opts in with its own DSN.
ARG VITE_SENTRY_DSN=""
ENV VITE_SENTRY_DSN=$VITE_SENTRY_DSN

# Optional: upload source maps for this release to Sentry (readable stack
# traces). Passed as a BuildKit secret, not a build-arg, so the token never
# lands in image layers/history. Skipped silently if not mounted.
RUN --mount=type=secret,id=sentry_auth_token \
    SENTRY_AUTH_TOKEN="$(cat /run/secrets/sentry_auth_token 2>/dev/null || true)" pnpm build


# ---- Stage 2: Build the Go backend ----
FROM golang:1.26-alpine AS backend
WORKDIR /app

RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY api ./api
COPY db ./db
COPY --from=frontend /app/dist ./dist

ARG VERSION=dev
RUN go build -ldflags="-X main.version=${VERSION}" -o fatura-cloud .


# ---- Stage 3: Minimal runtime image ----
FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

# Run as a non-root user with a fixed UID/GID (1000:1000) rather than a
# system-assigned one — /data is meant to be bind-mounted from a host
# directory in production, and the host side needs a stable UID to chown to
# that won't shift across image rebuilds. 1000 also matches the default
# first user on most Linux distros (including Raspberry Pi OS), so the host
# directory needs no chown at all in the common case.
RUN addgroup -g 1000 fatura && adduser -S -u 1000 -G fatura fatura \
    && mkdir -p /data \
    && chown -R fatura:fatura /app /data

COPY --from=backend --chown=fatura:fatura /app/fatura-cloud .

USER fatura

VOLUME ["/data"]
EXPOSE 8080

ENV PORT=8080

CMD ["./fatura-cloud"]
