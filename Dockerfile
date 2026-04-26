# Copyright 2026 Leonan Carvalho
# SPDX-License-Identifier: AGPL-3.0-only

# ---- Stage 1: Build ----
# BUILDPLATFORM uses the host OS/arch for the compiler (fast on multi-arch builds).
# TARGETOS/TARGETARCH are injected by `docker buildx build --platform`.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache module downloads before copying source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w -X main.serverVersion=${VERSION}" -o /docscout-mcp ./cmd/docscout/

# ---- Stage 2: Runtime ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S appgroup \
    && adduser  -S appuser -G appgroup

COPY --from=builder /docscout-mcp /usr/local/bin/docscout-mcp

# Persistent storage mount point for SQLite database.
RUN mkdir /data && chown appuser:appgroup /data
VOLUME ["/data"]

USER appuser

# Environment variables with defaults (override at runtime via -e or env file).
ENV GITHUB_TOKEN=""
ENV GITHUB_ORG=""
ENV SCAN_INTERVAL="30m"
ENV SCAN_FILES=""
ENV SCAN_DIRS=""
ENV EXTRA_REPOS=""
ENV REPO_TOPICS=""
ENV REPO_REGEX=""
ENV DATABASE_URL=""
ENV HTTP_ADDR=""
ENV MCP_HTTP_BEARER_TOKEN=""
ENV SCAN_CONTENT="false"
ENV MAX_CONTENT_SIZE="204800"
ENV GITHUB_WEBHOOK_SECRET=""

# Expose HTTP port for Streamable HTTP transport mode.
EXPOSE 8080

# Health check for HTTP transport mode. Override with --no-healthcheck for stdio mode.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz > /dev/null 2>&1 || exit 1

ENTRYPOINT ["docscout-mcp"]
