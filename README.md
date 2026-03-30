<div align="center">

# DocScout-MCP

![DocScout-MCP](docs/images/doc-scout-mcp-server.png)

DocScout-MCP is a **Model Context Protocol (MCP)** server written in Go that securely connects to your GitHub Organization, scans all repositories for documentation files, and provides intelligent context to AI Assistants (like Claude, Cursor, Antigravity, and others).

</div>

## Features

- **Automated Org-Wide Scanning**: Recursively searches repositories for documentation files. Target files and directories are fully customizable via environment variables.
- **Knowledge Graph Memory**: Built-in persistent memory powered by GORM (SQLite or PostgreSQL). AI agents can create entities, track relations, and add observations — surviving across sessions.
- **Flexible Transports**: Supports both **Stdio** (default) and **Streamable HTTP** transports.
- **Multi-Database Support**: Stores the knowledge graph in SQLite (file or in-memory) or PostgreSQL via `DATABASE_URL`.
- **Security First**: Defends against LLM hallucination and Path Traversal by _only_ allowing the AI to read files that were securely indexed as valid documentation.
- **Lightweight & Fast**: Built in Go with goroutines and semaphores for high-performance concurrent scanning.
- **Works with Orgs & Users**: Automatically detects whether the configured owner is an Organization or a personal account.

## Tools Exposed

### Scanner Tools

1. `list_repos`: Lists all repositories that contain documentation files.
2. `search_docs`: Searches documentation files by matching a query against file paths and repo names.
3. `get_file_content`: Retrieves the raw content of a specific documentation file.

### Knowledge Graph Tools

4. `create_entities`: Create nodes in the knowledge graph.
5. `create_relations`: Create directed edges between entities.
6. `add_observations`: Append facts to existing entities.
7. `delete_entities`: Remove entities (cascades relations and observations).
8. `delete_observations`: Remove specific observations from entities.
9. `delete_relations`: Remove specific relations.
10. `read_graph`: Read the entire knowledge graph.
11. `search_nodes`: Search entities by name, type, or observation content.
12. `open_nodes`: Retrieve specific entities by name with their relations.

## Security & GitHub Tokens 🔒

To run this server, you need a GitHub Personal Access Token (PAT).
**DO NOT use a Classic Token with broad scopes!**

### How to Create a Secure Token:

1. Go to GitHub -> Settings -> Developer Settings -> [Personal access tokens (Fine-grained)](https://github.com/settings/tokens?type=beta).
2. Click **Generate new token**.
3. Under **Resource owner**, select your target Organization.
4. Under **Repository access**, select "All repositories" (or specific ones).
5. Under **Repository permissions**, grant **Read-only** access to:
   - `Contents`
   - `Metadata`
6. Generate the token and use it for the `GITHUB_TOKEN` environment variable.

## Usage

### Environment Variables

| Variable        | Required | Default                                                                | Description                                                                                   |
| --------------- | -------- | ---------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `GITHUB_TOKEN`  | ✅       | —                                                                      | GitHub Personal Access Token (Fine-Grained)                                                   |
| `GITHUB_ORG`    | ✅       | —                                                                      | GitHub Organization or User name                                                              |
| `SCAN_INTERVAL` | ❌       | `30m`                                                                  | Re-scan interval. Supports Go duration format (`10s`, `5m`, `1h`) or plain integers (minutes) |
| `SCAN_FILES`    | ❌       | `catalog-info.yaml, mkdocs.yml, openapi.yaml, swagger.json, README.md` | Comma-separated filenames to scan at repo root                                                |
| `SCAN_DIRS`     | ❌       | `docs`                                                                 | Comma-separated directories to scan recursively for `.md` files                               |
| `EXTRA_REPOS`   | ❌       | —                                                                      | Comma-separated public/third-party repos to scan (e.g. `owner/repo`)                          |
| `REPO_TOPICS`   | ❌       | —                                                                      | Filter org repos by GitHub topics (e.g. `frontend, backend`)                                  |
| `REPO_REGEX`    | ❌       | —                                                                      | Filter org repos by regex matching the repo name (e.g. `^srv-.*`)                             |
| `DATABASE_URL`  | ❌       | In-memory SQLite                                                       | Knowledge graph storage. Accepts `sqlite://path.db` or `postgres://user:pass@host/db`         |
| `HTTP_ADDR`     | ❌       | —                                                                      | If set, starts Streamable HTTP transport at this address (e.g. `:8080`) instead of stdio      |

### 1. Running with Go (Stdio)

Requires Go 1.22+

```bash
export GITHUB_TOKEN="github_pat_11A..."
export GITHUB_ORG="my-awesome-org"
export SCAN_INTERVAL="1h"

go run .
```

### 2. Running with HTTP Transport

```bash
export GITHUB_TOKEN="github_pat_11A..."
export GITHUB_ORG="my-awesome-org"
export HTTP_ADDR=":8080"
export DATABASE_URL="sqlite://docscout.db"

go run .
# Server listening on http://localhost:8080
```

### 3. Running with Docker

```bash
# Stdio mode (default)
docker run -i \
  -e GITHUB_TOKEN="github_pat_11A..." \
  -e GITHUB_ORG="my-awesome-org" \
  ghcr.io/your-username/docscout-mcp:latest

# HTTP mode with persistent SQLite
docker run -p 8080:8080 \
  -e GITHUB_TOKEN="github_pat_11A..." \
  -e GITHUB_ORG="my-awesome-org" \
  -e HTTP_ADDR=":8080" \
  -e DATABASE_URL="sqlite:///data/kb.db" \
  -v docscout-data:/data \
  ghcr.io/your-username/docscout-mcp:latest

# HTTP mode with PostgreSQL
docker run -p 8080:8080 \
  -e GITHUB_TOKEN="github_pat_11A..." \
  -e GITHUB_ORG="my-awesome-org" \
  -e HTTP_ADDR=":8080" \
  -e DATABASE_URL="postgres://user:pass@db-host:5432/docscout" \
  ghcr.io/your-username/docscout-mcp:latest
```

## Client Configuration

See the [`docs/`](docs/) folder for detailed setup guides for each AI client:

- [VS Code (Copilot Chat)](docs/vscode.md)
- [GitHub Copilot](docs/copilot.md)
- [Antigravity (Google)](docs/antigravity.md)
- [Gemini CLI](docs/gemini.md)
- [ChatGPT Desktop](docs/chatgpt.md)

## Development

We welcome contributions! Please make sure to review the official Development Guidelines before submitting any code:

- **[Development Guidelines for Humans](docs/DEVELOPMENT_GUIDELINES.md)**
- **AI Assistants:** This repository includes an `AGENTS.md` file that configures AI agents (like Cursor, Copilot, or Antigravity) with the exact constraints needed for the official MCP Go SDK.

```bash
# Install dependencies
go mod tidy

# Build
go build -o docscout-mcp .

# Test (unit + E2E integration)
go test ./...
```

## License

GNU AGPL v3
