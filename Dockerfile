# ---- Stage 1: Build ----
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /docscout-mcp .

# ---- Stage 2: Runtime ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /docscout-mcp /usr/local/bin/docscout-mcp

# Environment variables (override at runtime).
ENV GITHUB_TOKEN=""
ENV GITHUB_ORG=""
ENV SCAN_INTERVAL="30m"
ENV SCAN_FILES=""
ENV SCAN_DIRS=""

ENTRYPOINT ["docscout-mcp"]
