# syntax=docker/dockerfile:1

# ---- Stage 1: Build the React frontend ----
FROM node:22-alpine AS frontend
WORKDIR /app

RUN npm install -g pnpm

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY index.html lingui.config.ts vite.config.ts tsconfig.json tsconfig.node.json ./
COPY src ./src

RUN pnpm build


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

COPY --from=backend /app/fatura-cloud .

VOLUME ["/data"]
EXPOSE 8080

ENV PORT=8080

CMD ["./fatura-cloud"]
