# syntax=docker/dockerfile:1

# ---- Stage 1: Build the React frontend ----
# Pinned to the build host's own platform, not the target one: this stage's
# output (static JS/CSS/HTML) is architecture-independent, so building it
# under arm64 QEMU emulation on a multi-arch buildx run would just be a much
# slower way to produce the exact same files.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /app

# corepack (bundled with Node 22) reads package.json's own "packageManager"
# pin and fetches that exact pnpm release — `npm install -g pnpm` instead
# grabs pnpm's single-executable-application build, whose per-platform
# @pnpm/exe.* native binary is an optionalDependency npm doesn't reliably
# install (fails with "no @pnpm/exe.<platform> native binary was found for
# this host" on some npm/arch combinations); corepack's shim has no such gap.
RUN corepack enable

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
# Also pinned to the build host's own platform, same reasoning as the
# frontend stage: the only Go dependency that ever needed a C toolchain was
# mattn/go-sqlite3, and this app uses modernc.org/sqlite (pure Go, no cgo)
# instead — see db/db.go. With CGO_ENABLED=0, `go build` cross-compiles for
# TARGETARCH natively, no QEMU emulation needed here either.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY api ./api
COPY db ./db
COPY --from=frontend /app/dist ./dist

ARG VERSION=dev
ARG TARGETARCH
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH GOOS=linux go build -ldflags="-X main.version=${VERSION}" -o fatura-cloud .


# ---- Stage 3: Runtime image ----
# Debian, not Alpine: libreoffice-calc (needed for the PDF export path —
# db/pdf_convert.go, GET /api/invoices/{id}/export?format=pdf) crashes on
# startup on Alpine/musl (`terminate called after throwing an instance of
# 'com::sun::star::uno::RuntimeException'`, unfixed by every standard
# workaround — see git history for the abandoned attempt) but runs cleanly
# on Debian, headless and as a non-root user, verified directly against this
# exact soffice invocation on both amd64 and arm64 (the latter matching a
# Raspberry Pi deployment target). The trade is image size: ~700MB here vs.
# ~80MB on Alpine, almost entirely libreoffice-calc and its own
# dependencies — accepted deliberately so PDF export actually works in the
# shipped image instead of always 503ing.
FROM debian:bookworm-slim
WORKDIR /app

# --no-install-recommends keeps this to libreoffice-calc's own dependency
# tree, not the full LibreOffice suite's optional extras (Writer/Impress
# filters, spell-check dictionaries, etc. this app never touches).
# fonts-liberation is metric-compatible with Arial/Times New Roman/Courier —
# an org's uploaded .xlsx template (issue #115) can specify any font, and
# without a reasonable substitute installed LibreOffice's fallback can shift
# column widths enough to visibly misalign a template that looked fine in
# Excel. fonts-dejavu-core covers the rest as a general-purpose fallback.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        libreoffice-calc \
        fonts-liberation \
        fonts-dejavu-core \
    && rm -rf /var/lib/apt/lists/*

# Run as a non-root user with a fixed UID/GID (1000:1000) rather than a
# system-assigned one — /data is meant to be bind-mounted from a host
# directory in production, and the host side needs a stable UID to chown to
# that won't shift across image rebuilds. 1000 also matches the default
# first user on most Linux distros (including Raspberry Pi OS), so the host
# directory needs no chown at all in the common case. `-M` skips creating a
# home directory (soffice's per-request profile lives under a temp dir
# instead, see db/pdf_convert.go's `-env:UserInstallation`, not `~`).
RUN groupadd -g 1000 fatura && useradd -M -u 1000 -g fatura fatura \
    && mkdir -p /data \
    && chown -R fatura:fatura /app /data

COPY --from=backend --chown=fatura:fatura /app/fatura-cloud .

USER fatura

VOLUME ["/data"]
EXPOSE 8080

ENV PORT=8080

CMD ["./fatura-cloud"]
