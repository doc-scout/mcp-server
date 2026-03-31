# DocScout-MCP Roadmap

This document outlines the current technical debts and the path forward for DocScout-MCP to become a more resilient, intelligent, and widely adopted open-source tool.

## Completed

### 1. Incremental Ingestion Pipeline (Event-Driven) ✅
- **Implemented**: GitHub Webhook support via opt-in `GITHUB_WEBHOOK_SECRET` env var (requires `HTTP_ADDR`).
- `webhook/webhook.go` — validates `X-Hub-Signature-256` HMAC-SHA256, handles `push`, `create`, `delete`, and `repository` events. Ignores irrelevant events (`ping`, `star`, `issues`) with a `200 OK`.
- `scanner/scanner.go` — `TriggerRepoScan` performs a targeted single-repo scan and invokes the indexer callback, bypassing the full org polling cycle.
- Bearer token auth middleware explicitly excludes `/webhook` since it carries its own HMAC auth.

### 3. Rate Limiting, Resilience, and Circuit Breakers ✅
- **Implemented**: `scanner/retry.go` — `retryGitHub` wraps every GitHub API call site with up to 3 retries and smart wait strategies:
  - Primary rate limit (`*RateLimitError`): waits until `Rate.Reset`, capped at 5 minutes.
  - Secondary/abuse rate limit (`*AbuseRateLimitError`): respects `Retry-After` header.
  - Transient 5xx / 429: exponential backoff (2s → 4s → 8s).
  - Non-retryable errors (4xx, context cancellation): returned immediately.
- All five GitHub API call sites in the scanner are wrapped: `ListByOrg`, `ListByUser`, `Repositories.Get` (extra repos), `GetContents` (per-file scan and directory scan), and `GetFileContent`.

### 6. Auto-Discovery (AST & Dependencies Parsing) ✅
- **Implemented**: Automatic service entity and dependency graph inference from manifest files, without requiring a Backstage `catalog-info.yaml`.
- `scanner/parser/gomod.go` — `ParseGoMod` extracts module path, Go version, and direct (non-indirect) dependencies from `go.mod`. `go.mod` added to `DefaultTargetFiles`.
- `scanner/parser/packagejson.go` — `ParsePackageJSON` extracts name, version, and runtime `dependencies` (excluding `devDependencies`) from `package.json`. Scoped names (`@org/pkg`) are normalized to `pkg`. `package.json` added to `DefaultTargetFiles`.
- `scanner/parser/pom.go` — `ParsePom` extracts `groupId`, `artifactId`, `version`, and compile/runtime-scope dependencies from `pom.xml`. `test` and `provided` scopes are excluded. `pom.xml` added to `DefaultTargetFiles`.
- `scanner/parser/codeowners.go` — `ParseCodeowners` extracts all unique owners from `CODEOWNERS` files. Supports `@org/team` (→ `team` entity), `@username` (→ `person` entity), and `user@email.com` formats. Checks three GitHub-supported locations: `CODEOWNERS`, `.github/CODEOWNERS`, `docs/CODEOWNERS`.
- `indexer/indexer.go` — Phases 2b–2e auto-upsert entities with source observations and `depends_on` / `owns` relations.

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


### 2. Semantic Search and Vector Embeddings (RAG)
- **Current State**: Content search relies on exact text matching (`LIKE` queries in SQL).
- **Goal**: Integrate vector embeddings (e.g., using `pgvector` for PostgreSQL or `sqlite-vss`) to allow AI Assistants to perform true semantic searches. This will drastically improve the relevance of the retrieved context.

### 4. Graph Knowledge Access Control (RBAC)
- **Current State**: Any LLM client connected to the MCP server can read any file and entity that was indexed.
- **Goal**: Implement Role-Based Access Control (RBAC) so that sensitive architectural or security documents are only accessible to authorized users or service accounts.

### 5. Multi-Cloud and Platform Adapters
- **Current State**: Hardcoded dependency on GitHub API.
- **Goal**: Build a generic "Provider" interface to support GitLab, Bitbucket, Confluence, Notion, and other internal enterprise wikis out-of-the-box.


### 7. Deployment and Operations
- **Current State**: Manual deployment via `go run` or raw Docker commands.
- **Goal**: Create production-ready deployment assets ("One-click deploy" Helm charts, Terraform modules, and K8s manifests).

### 8. Observability — Remaining
- **Current State**: Tool call counts are tracked. Per-document access is not yet measured.
- **Goal**: Track which specific files are fetched most via `get_file_content` and `search_content`. Add Grafana dashboard templates and alerting rules for rate limit exhaustion.

### 9. Knowledge Graph Protection — Remaining
- **Current State**: Mass-deletion is guarded. Hallucination detection is not implemented.
- **Goal**: Add heuristics to detect low-quality or duplicate observations on `create_entities` and `add_observations`. Consider an audit log of all graph mutations.
