# DocScout-MCP

DocScout-MCP is a **Model Context Protocol (MCP)** server written in Go that securely connects to your GitHub Organization, scans all repositories for documentation files, and provides intelligent context to AI Assistants (like Claude, Cursor, and others).

## Features

- **Automated Org-Wide Scanning**: Recursively searches repositories for target documentation files on an interval (`catalog-info.yaml`, `mkdocs.yml`, `openapi.yaml`, `swagger.json`, `README.md`, and everything under `docs/**/*.md`).
- **In-Memory Caching**: Prevents aggressive GitHub API Rate Limits by indexing your org's structure in memory.
- **Security First**: Defends against LLM hallucination and Path Traversal by *only* allowing the AI to read files that were securely indexed as valid documentation. 
- **Lightweight & Fast**: Built in Go, utilizing goroutines and semaphores for high-performance concurrent repository scanning.

## Tools Exposed

DocScout-MCP exposes the following tools to the AI:

1. `list_repos`: Lists all repositories in the organization that contain documentation files.
2. `search_docs`: Searches for documentation files by matching a query term against file paths and repository names.
3. `get_file_content`: Retrieves the raw markdown/yaml content of a specific documentation file.

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

### 1. Running with Go

Requires Go 1.22+

```bash
export GITHUB_TOKEN="github_pat_11A..."
export GITHUB_ORG="my-awesome-org"
# Optional: Change the background scan interval (Default is 30 minutes)
export SCAN_INTERVAL="60"

go run .
```

### 2. Running with Docker

```bash
docker run -i \
  -e GITHUB_TOKEN="github_pat_11A..." \
  -e GITHUB_ORG="my-awesome-org" \
  -e SCAN_INTERVAL="30" \
  ghcr.io/your-username/docscout-mcp:latest
```

## Client Configuration

### Claude Desktop

Add this to your `claude_desktop_config.json`:

**If using Go directly:**
```json
{
  "mcpServers": {
    "docscout": {
      "command": "/path/to/docscout-mcp/binary",
      "env": {
        "GITHUB_TOKEN": "github_pat_...",
        "GITHUB_ORG": "my-awesome-org"
      }
    }
  }
}
```

**If using Docker:**
```json
{
  "mcpServers": {
    "docscout": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e",
        "GITHUB_TOKEN=github_pat_...",
        "-e",
        "GITHUB_ORG=my-awesome-org",
        "ghcr.io/your-username/docscout-mcp:latest"
      ]
    }
  }
}
```

## Development

```bash
# Install dependencies
go mod tidy

# Build
go build -o docscout-mcp .

# Test
go test ./...
```

## License

MIT
