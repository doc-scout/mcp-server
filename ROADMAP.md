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

### 8. Observability and Metrics (Prometheus) ✅
- **Implemented**: Per-tool call counters and per-document access tracking, exposed via MCP and HTTP.
- `tools/metrics.go` — `ToolMetrics` (per-tool counters) and `DocMetrics` (per-document access counters), both thread-safe via `sync.RWMutex` + `atomic.Int64`.
- `tools/tools.go` — `withMetrics` wrapper instruments all registered tools. `get_file_content` and `search_content` both record document accesses via `DocMetrics`.
- `tools/get_usage_stats.go` — `get_usage_stats` MCP tool returns tool call counts **and** the top 20 most-fetched documents since server start.
- `main.go` — `/metrics` HTTP endpoint emits two Prometheus counters: `docscout_tool_calls_total{tool}` and `docscout_document_accesses_total{repo,path}`.

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


### 7. Deployment and Operations ✅
- **Implemented**: Full production-ready deployment suite across multiple targets.
- `Dockerfile` — multi-stage, multi-arch (`linux/amd64`, `linux/arm64`), non-root user, HEALTHCHECK, all env vars declared.
- `docker-compose.yml` — three profiles: `http` (SQLite, default), `postgres` (PostgreSQL backend), `stdio` (MCP Inspector / Claude Desktop).
- `Makefile` — `build`, `test`, `lint`, `docker-build`, `docker-build-multiarch`, `compose-up`, `k8s-deploy`, `helm-install`, `release` targets and more.
- `.mise.toml` — extended with `docker-build`, `compose-up/down`, `helm-lint`, `helm-template`, `clean` tasks.
- `deploy/k8s/` — raw Kubernetes manifests: Namespace, Secret, ConfigMap, PVC, Deployment (non-root, probes, resource limits), Service, Ingress.
- `deploy/helm/` — full Helm chart v2 with `values.yaml`, `_helpers.tpl`, and templates for all resources (Deployment, Service, ConfigMap, Secret, PVC, Ingress).
- `deploy/terraform/` — Kubernetes Terraform module (`hashicorp/kubernetes` provider): Namespace, Secret, ConfigMap, PVC, Deployment, Service, optional Ingress. Works with any K8s cluster (EKS, GKE, AKS, local).


### 9. Knowledge Graph Protection — Remaining
- **Current State**: Mass-deletion is guarded. Hallucination detection is not implemented.
- **Goal**: Add heuristics to detect low-quality or duplicate observations on `create_entities` and `add_observations`. Consider an audit log of all graph mutations.
