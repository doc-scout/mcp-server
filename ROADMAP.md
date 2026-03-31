# DocScout-MCP Roadmap

This document outlines the current technical debts and the path forward for DocScout-MCP to become a more resilient, intelligent, and widely adopted open-source tool.

## Completed

### 3. Rate Limiting, Resilience, and Circuit Breakers ✅
- **Implemented**: `scanner/retry.go` — `retryGitHub` wraps every GitHub API call site with up to 3 retries and smart wait strategies:
  - Primary rate limit (`*RateLimitError`): waits until `Rate.Reset`, capped at 5 minutes.
  - Secondary/abuse rate limit (`*AbuseRateLimitError`): respects `Retry-After` header.
  - Transient 5xx / 429: exponential backoff (2s → 4s → 8s).
  - Non-retryable errors (4xx, context cancellation): returned immediately.
- All five GitHub API call sites in the scanner are wrapped: `ListByOrg`, `ListByUser`, `Repositories.Get` (extra repos), `GetContents` (per-file scan and directory scan), and `GetFileContent`.

### 6. Auto-Discovery (AST & Dependencies Parsing) ✅ *(partial)*
- **Implemented**: Automatic service entity and dependency graph inference from manifest files, without requiring a Backstage `catalog-info.yaml`.
- `scanner/parser/gomod.go` — `ParseGoMod` extracts module path, Go version, and direct (non-indirect) dependencies from `go.mod`. `go.mod` added to `DefaultTargetFiles`.
- `scanner/parser/packagejson.go` — `ParsePackageJSON` extracts name, version, and runtime `dependencies` (excluding `devDependencies`) from `package.json`. Scoped names (`@org/pkg`) are normalized to `pkg`. `package.json` added to `DefaultTargetFiles`.
- `indexer/indexer.go` — Phase 2b (go.mod) and Phase 2c (package.json) auto-upsert `service` entities with source observations and `depends_on` relations to each direct dependency.
- **Remaining**: `pom.xml` (Maven/Java) and `CODEOWNERS` (ownership inference) parsers.

### 8. Observability and Metrics (Prometheus) ✅ *(partial)*
- **Implemented**: Per-tool call counters exposed via MCP and HTTP.
- `tools/metrics.go` — `ToolMetrics`: thread-safe call counter using `sync.RWMutex` + `atomic.Int64`.
- `tools/tools.go` — `withMetrics` wrapper increments the counter before each tool invocation. All registered tools are instrumented.
- `tools/get_usage_stats.go` — New `get_usage_stats` MCP tool returns a snapshot of call counts since server start. AI agents can call this to identify the most-accessed documentation.
- `main.go` — `/metrics` HTTP endpoint (when `HTTP_ADDR` is set) emits Prometheus text format (`docscout_tool_calls_total` counter with `tool` label).
- **Remaining**: Alerting rules, Grafana dashboards, per-document access tracking.

### 9. Knowledge Graph Protection ✅ *(partial)*
- **Implemented**: Mass-deletion guard on `delete_entities`.
- `tools/delete_entities.go` — Requests deleting more than 10 entities in a single call are rejected unless `confirm: true` is explicitly set. The threshold (`massDeleteThreshold = 10`) is a named constant. The tool description surfaces this requirement to AI agents.
- **Remaining**: Hallucination detection heuristics, moderation for `create_entities` (duplicate/low-quality observation filtering).

---

## Future Work

### 1. Incremental Ingestion Pipeline (Event-Driven)
- **Current State**: The scanner uses polling (`SCAN_INTERVAL`) to fetch data from GitHub repositories, which can lead to unnecessary API calls and rate-limiting issues on large organizations.
- **Goal**: Implement GitHub Webhook integrations (Push, Release events) to trigger targeted, real-time scans of only the modified files, saving resources and ensuring the Knowledge Graph is always instantly up to date.

### 2. Semantic Search and Vector Embeddings (RAG)
- **Current State**: Content search relies on exact text matching (`LIKE` queries in SQL).
- **Goal**: Integrate vector embeddings (e.g., using `pgvector` for PostgreSQL or `sqlite-vss`) to allow AI Assistants to perform true semantic searches. This will drastically improve the relevance of the retrieved context.

### 4. Graph Knowledge Access Control (RBAC)
- **Current State**: Any LLM client connected to the MCP server can read any file and entity that was indexed.
- **Goal**: Implement Role-Based Access Control (RBAC) so that sensitive architectural or security documents are only accessible to authorized users or service accounts.

### 5. Multi-Cloud and Platform Adapters
- **Current State**: Hardcoded dependency on GitHub API.
- **Goal**: Build a generic "Provider" interface to support GitLab, Bitbucket, Confluence, Notion, and other internal enterprise wikis out-of-the-box.

### 6. Auto-Discovery — Remaining Parsers
- **Current State**: `go.mod` and `package.json` are parsed. `pom.xml` and `CODEOWNERS` are not yet handled.
- **Goal**: Add `pom.xml` (Maven dependency graph for Java services) and `CODEOWNERS` (map file ownership to team entities) parsers following the same pattern.

### 7. Deployment and Operations
- **Current State**: Manual deployment via `go run` or raw Docker commands.
- **Goal**: Create production-ready deployment assets ("One-click deploy" Helm charts, Terraform modules, and K8s manifests).

### 8. Observability — Remaining
- **Current State**: Tool call counts are tracked. Per-document access is not yet measured.
- **Goal**: Track which specific files are fetched most via `get_file_content` and `search_content`. Add Grafana dashboard templates and alerting rules for rate limit exhaustion.

### 9. Knowledge Graph Protection — Remaining
- **Current State**: Mass-deletion is guarded. Hallucination detection is not implemented.
- **Goal**: Add heuristics to detect low-quality or duplicate observations on `create_entities` and `add_observations`. Consider an audit log of all graph mutations.
