# Contributing to DocScout-MCP

Thank you for your interest in contributing. DocScout-MCP is an open-source MCP server licensed under AGPL-3.0.

---

## Try DocScout in 60 Seconds

No Git clone needed:

```sh
curl -fsSL https://raw.githubusercontent.com/doc-scout/mcp-server/main/bin/docscout-init.sh | sh
```

The script downloads the latest binary, writes `.env.local`, and prints the Claude Desktop config snippet.

---

## Getting Started

### Prerequisites

- **Go 1.26+** — `go version` must report `go1.26` or later
- A **GitHub Fine-Grained PAT** with read-only `Contents` + `Metadata` access (for integration testing against a real org)

### Local Setup

```bash
git clone https://github.com/doc-scout/mcp-server
cd mcp-server

# Install dependencies
go mod download

# Build
go build -o docscout-mcp ./cmd/docscout/

# Run unit + E2E tests
go test ./...
```

### Run in Development

```bash
GITHUB_TOKEN="github_pat_..." \
GITHUB_ORG="my-org" \
SCAN_INTERVAL="2m" \
go run ./cmd/docscout/
```

---

## Project Structure

```
.
├── cmd/
│   ├── docscout/             # Main entrypoint (Wire → Run)
│   └── report/               # Standalone graph-report CLI
├── internal/
│   ├── core/                 # Domain layer — no external deps
│   │   ├── graph/            # Entity, Relation, port interfaces
│   │   ├── audit/            # AuditStore port + AuditEvent model
│   │   ├── content/          # ContentRepository port
│   │   └── scan/             # RepoInfo, FileEntry models
│   ├── infra/                # Outbound adapters
│   │   ├── db/               # SQLite/Postgres (GORM)
│   │   ├── github/           # GitHub API scanner
│   │   │   └── parser/       # Manifest parsers (go.mod, pom.xml, …)
│   │   └── embeddings/       # Vector store + semantic search
│   ├── adapter/
│   │   ├── mcp/              # MCP tool handlers (inbound)
│   │   └── http/             # HTTP health/webhook handlers (inbound)
│   └── app/                  # Composition root (wire.go, server.go, …)
├── tests/                    # E2E integration tests (one dir per tool)
└── docs/                     # This documentation site
```

---

## Development Guidelines

Read [Development Guidelines](DEVELOPMENT_GUIDELINES.md) and `AGENTS.md` in the repo root before submitting a PR. Key constraints:

!!! danger "STDIO safety"
**Never** use `fmt.Println`, `fmt.Printf`, or anything that writes to `os.Stdout`. The MCP server communicates via JSON-RPC over stdio — free text on stdout corrupts the stream. Use `log/slog` (writes to stderr) instead.

!!! warning "Input validation"
All inputs from LLM clients are untrusted. Validate against the internal index before passing to `os.ReadFile` or GitHub API calls.

!!! info "New parsers"
New manifest parsers go in `internal/infra/github/parser/` and implement the `FileParser` interface. Register them via `parser.Register()` in `internal/app/wire.go`.

---

## Testing

```bash
# Run all tests
go test ./...

# Run a specific package with verbose output
go test ./internal/adapter/mcp/... -v

# Run a specific test
go test ./tests/traverse_graph/... -run TestE2E_TraverseGraph_Incoming -v

# Race detector
go test -race ./...
```

Every PR must pass the full test suite. New features must include:

- **Unit tests** for any new parser or memory layer function
- **E2E tests** under `tests/<tool_name>/` for any new MCP tool (use `testutils.SetupTestServer`)

---

## Pull Request Checklist

- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes (including race detector: `go test -race ./...`)
- [ ] `go vet ./...` passes with no warnings
- [ ] All new files include the license header:
  ```go
  // Copyright 2026 Leonan Carvalho
  // SPDX-License-Identifier: AGPL-3.0-only
  ```
- [ ] Tool descriptions are detailed enough for an LLM to choose the right tool
- [ ] No `fmt.Print*` writing to stdout
- [ ] If adding a new MCP tool: handler in `internal/adapter/mcp/`, registered in `internal/app/server.go`, mock updated in `tests/testutils/utils.go`

---

## Reporting Issues

Use [GitHub Issues](https://github.com/doc-scout/mcp-server/issues) for bugs and feature requests.

For **security vulnerabilities**, see [Security](security.md) — do not open a public issue.
