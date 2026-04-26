# How DocScout-MCP Works

DocScout-MCP bridges the gap between large, distributed software systems and AI agents. It creates a reliable, deterministic model of your architecture and exposes it — alongside raw documentation content — to any LLM via the Model Context Protocol (MCP).

This document explains the three core mechanisms: **Scanning & Parsing**, the **Deterministic Knowledge Graph**, and the **MCP Interface**.

---

## 1. Scanning & Parsing

The scanner (`scanner/scanner.go`) runs on startup and repeats at every `SCAN_INTERVAL`. It discovers three categories of files:

### 1a. Root-Level Target Files

Checked at every repository root via the GitHub Contents API:

| File | Type | What is extracted |
|------|------|-------------------|
| `catalog-info.yaml` | `catalog-info` | Backstage entity, relations, owner, lifecycle |
| `go.mod` | `gomod` | Module path, Go version, direct dependencies |
| `package.json` | `packagejson` | Package name, version, runtime dependencies |
| `pom.xml` | `pomxml` | Maven groupId, artifactId, version, compile/runtime deps |
| `CODEOWNERS` / `.github/CODEOWNERS` / `docs/CODEOWNERS` | `codeowners` | Owners per path pattern (team, person, email) |
| `Dockerfile` | `dockerfile` | Presence of containerisation |
| `docker-compose.yml` / `docker-compose.yaml` | `compose` | Multi-service composition |
| `Makefile` | `makefile` | Build automation presence |
| `.mise.toml` / `mise.toml` | `mise` | Tool version management |
| `AGENTS.md` | `agents` | AI agent instructions surface |
| `SKILLS.md` | `skills` | AI agent skills surface |
| `README.md`, `mkdocs.yml`, `openapi.yaml`, `swagger.json` | `readme`, `mkdocs`, `openapi`, `swagger` | Documentation surface |

### 1b. Documentation Directories

Directories listed in `SCAN_DIRS` (default: `docs/`, `.agents/`) are scanned recursively for `.md` files, typed as `docs`.

### 1c. Infrastructure Directories

Directories listed in `SCAN_INFRA_DIRS` (default: `deploy/`, `infra/`, `.github/workflows/`) are scanned recursively for infrastructure files:

| Extension | Type assigned |
|-----------|--------------|
| `Chart.yaml` or any `.yaml` under `/helm/` | `helm` |
| Any `.yaml`/`.yml` under `/k8s/` or `/kubernetes/` | `k8s` |
| Any `.yaml`/`.yml` under `/workflows/` | `workflow` |
| `*.tf`, `*.hcl` | `terraform` |
| `*.toml` | `toml` |
| Other `.yaml`/`.yml` | `infra` |

### 1d. Incremental Scans via Webhooks (optional)

When `GITHUB_WEBHOOK_SECRET` is set, the `/webhook` endpoint is activated. GitHub sends a signed `POST` for `push`, `create`, `delete`, and `repository` events. The server verifies `X-Hub-Signature-256` (HMAC-SHA256) and immediately triggers a targeted single-repo scan, bypassing the full org polling cycle. Unrelated events (`ping`, `star`, `issues`) are acknowledged and ignored.

---

## 2. The Deterministic Knowledge Graph

Rather than letting the AI infer relationships, the **AutoIndexer** (`internal/app/indexer.go`) translates parsed manifests into graph entities, relations, and observations stored in SQLite or PostgreSQL.

### Indexer Phases

After each scan completes, the indexer runs sequentially:

| Phase | Source | Graph output |
|-------|--------|-------------|
| 1 | All files | Content cache refresh (if `SCAN_CONTENT=true`) |
| 2 | `catalog-info.yaml` | Entity + type + observations + explicit relations |
| 2b | `go.mod` | `service` entity, `go_module`, `go_version`, `depends_on` per direct dep |
| 2c | `package.json` | `service` entity, `npm_package`, `version`, `depends_on` per runtime dep |
| 2d | `pom.xml` | `service` entity, `maven_artifact`, `java_group`, `version`, `depends_on` per compile/runtime dep |
| 2e | `CODEOWNERS` | `team`/`person` entities per owner, `owns` relations to the service entity |
| 2f | `.mcp.json`, `claude_desktop_config.json`, `.cursor/mcp.json`, `mcp.json`, `.vscode/mcp.json` | `mcp-server` entities per declared server, `uses_mcp` relation from the repo service to each server |
| 3 | All entities | Entities from repos no longer in the scan receive `_status:archived` |

### Graph Safety

Every write operation passes through two layers of protection:

1. **Observation quality filter** (`tools/graph_guard.go`): Rejects observations that are empty, shorter than 2 characters, longer than 500 characters, or duplicate within the same batch. The tool response includes a `skipped` field listing each rejection with its reason.

2. **Audit logger** (`tools/audit.go`): A `GraphAuditLogger` decorator wraps the store, emits a structured `slog.Info` line for every mutation, and — when `DATABASE_URL` points to a persistent store — writes a row to the `audit_events` table. Each row records the agent identity (resolved from `AGENT_ID` env → MCP handshake `clientInfo.Name` → `"unknown"`), the tool name, operation, targets, count, and outcome. Read-only operations pass through silently.

3. **Mass-delete guard** (`tools/delete_entities.go`): Deleting more than 10 entities in a single call is rejected unless `confirm: true` is explicitly set.

### Example: Inferring a Service from `go.mod`

```
module github.com/myorg/payment-service

go 1.26

require (
    github.com/myorg/billing-lib v1.2.0
    github.com/jackc/pgx/v5 v5.9.1
)
```

The indexer produces:
- **Entity**: `payment-service` (type: `service`)
- **Observations**: `_source:go.mod`, `_scan_repo:myorg/payment-service`, `go_module:github.com/myorg/payment-service`, `go_version:1.26`
- **Relations**: `payment-service → depends_on → billing-lib`, `payment-service → depends_on → pgx`

### Example: Inferring Ownership from `CODEOWNERS`

```
# CODEOWNERS
/services/payment/ @myorg/payments-team
*.go               @alice
```

The indexer produces:
- **Entity**: `payments-team` (type: `team`), observation: `github_handle:@myorg/payments-team`
- **Entity**: `alice` (type: `person`), observation: `github_handle:@alice`
- **Relations**: `payments-team → owns → payment-service`, `alice → owns → payment-service`

---

## 3. The MCP Interface

The server exposes tools over `stdio` (default) or Streamable HTTP (`HTTP_ADDR`). All tools are instrumented with call-count metrics.

### Scanner Tools

| Tool | Description |
|------|-------------|
| `list_repos` | Lists all repos that contain indexed files |
| `search_docs` | Searches file paths and repo names by query |
| `get_file_content` | Fetches raw content of an indexed file (path-traversal protected) |
| `get_scan_status` | Returns scanner state, last scan time, repo count, content cache size |
| `search_content` | Full-text search across cached documentation content (available when `SCAN_CONTENT=true`) |

### Knowledge Graph Tools

| Tool | Description |
|------|-------------|
| `create_entities` | Create nodes; observations are sanitized before storage |
| `create_relations` | Create directed edges between entities |
| `add_observations` | Append facts to existing entities (sanitized, deduplicated) |
| `delete_entities` | Remove entities (cascades relations/observations; > 10 requires `confirm: true`) |
| `delete_observations` | Remove specific observations |
| `delete_relations` | Remove specific relations |
| `read_graph` | Return the full knowledge graph |
| `search_nodes` | Search entities by name, type, or observation content |
| `open_nodes` | Retrieve specific entities by name with their relations |

### Observability Tools

| Tool | Description |
|------|-------------|
| `get_usage_stats` | Returns per-tool call counts + top 20 most-fetched documents since server start |

The `/metrics` HTTP endpoint (when `HTTP_ADDR` is set) emits two Prometheus counters:
- `docscout_tool_calls_total{tool}` — total calls per tool
- `docscout_document_accesses_total{repo,path}` — total fetches per document

### Audit Tools (requires persistent `DATABASE_URL`)

| Tool | Description |
|------|-------------|
| `query_audit_log` | Query the persistent audit log by agent, tool, operation, outcome, or time window |
| `get_audit_summary` | Aggregated governance report with risky event detection (mass deletes, unknown agents, error bursts) |

The `/audit` and `/audit/summary` HTTP endpoints (when `HTTP_ADDR` is set) expose the same data for dashboards and operators.

### MCP Discovery Tools

| Tool | Description |
|------|-------------|
| `discover_mcp_servers` | Inventory all `mcp-server` entities in the graph; filter by repo, transport, or tool capability |

---

## Practical Example: Answering Architectural Queries

> *"What happens if I shut down `component:db`? Which systems will go offline, and who should I notify?"*

1. **`search_nodes("component:db")`** — finds `component:db` and a `depends_on` edge from `payment-service`
2. **`open_nodes(["payment-service"])`** — reveals observations: `owner: team-payments`, `_source:go.mod`, `go_version:1.26`
3. **`search_nodes("team-payments")`** — finds the `team` entity with `owns → payment-service` relation and a `github_handle:@myorg/payments-team` observation

The agent responds with verified facts from the graph — not hallucinations based on file naming conventions.
