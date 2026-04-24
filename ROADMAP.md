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

### 9. Knowledge Graph Protection ✅
- **Implemented**: Mass-deletion guard, observation quality filtering, and full mutation audit log.
- `tools/delete_entities.go` — Requests deleting more than 10 entities in a single call are rejected unless `confirm: true` is explicitly set.
- `tools/graph_guard.go` — `sanitizeObservations` filters observations before any write: rejects empty/whitespace-only, too-short (< 2 chars), too-long (> 500 chars), and deduplicates within the batch. Both `create_entities` and `add_observations` return a `skipped` field listing every rejection with its reason.
- `tools/audit.go` — `GraphAuditLogger` decorator wraps the entire `GraphStore`. Every mutation (`create_entities`, `add_observations`, `create_relations`, `delete_*`) emits a structured `slog.Info` line with entity names and counts. Read-only operations pass through silently. The logger is applied globally in `main.go`, covering both AI agent calls and the auto-indexer.

### 10. Architecture Discovery & Content Search ✅
- **Implemented**: Automatic knowledge graph population from `catalog-info.yaml` Backstage manifests and opt-in full-text content search across all indexed documentation.
- `scanner/parser/catalog.go` — `ParseCatalog` extracts entity name, type (mapped from Backstage `kind`/`spec.type`), lifecycle, description, tags, and all spec relations (`owned_by`, `part_of`, `depends_on`, `provides_api`, `consumes_api`). Entity names are validated against a strict `[a-zA-Z0-9._-]{1,253}` regex, rejecting unsafe values.
- `memory/content.go` — `ContentCache` persists raw file content in a `doc_contents` GORM table with SHA-based incremental diffing. Files larger than `MAX_CONTENT_SIZE` (default 200 KB) are skipped. Full-text search uses `LIKE`/`ILIKE` with proper `%` and `_` wildcard escaping to prevent injection. Enabled only when `SCAN_CONTENT=true` on a persistent `DATABASE_URL`.
- `indexer/indexer.go` — Phase 2a upserts catalog entities, observations, and relations; honours a three-tier merge strategy: create-new, update-auto (`_source:catalog-info`), or add-missing-only (manual entities). Phase 1 refreshes the content cache. Phase 3 soft-deletes stale entities with `_status:archived`.
- `scanner/scanner.go` — `SetOnScanComplete` callback wires the `AutoIndexer` into the scanner lifecycle without coupling the packages.
- `tools/search_content.go` — new `search_content` MCP tool for full-text search across cached documentation; returns repo name, path, and a ≤300-char context snippet per match (max 20 results). Returns a clear error if content caching is disabled.
- `tools/get_scan_status.go` — new `get_scan_status` MCP tool exposing scanner state, last scan time, repo count, content cache size, graph entity count, and whether `SCAN_CONTENT` is enabled.

### 7. Deployment and Operations ✅
- **Implemented**: Full production-ready deployment suite across multiple targets.
- `Dockerfile` — multi-stage, multi-arch (`linux/amd64`, `linux/arm64`), non-root user, HEALTHCHECK, all env vars declared.
- `docker-compose.yml` — three profiles: `http` (SQLite, default), `postgres` (PostgreSQL backend), `stdio` (MCP Inspector / Claude Desktop).
- `Makefile` — `build`, `test`, `lint`, `docker-build`, `docker-build-multiarch`, `compose-up`, `k8s-deploy`, `helm-install`, `release` targets and more.
- `.mise.toml` — extended with `docker-build`, `compose-up/down`, `helm-lint`, `helm-template`, `clean` tasks.
- `deploy/k8s/` — raw Kubernetes manifests: Namespace, Secret, ConfigMap, PVC, Deployment (non-root, probes, resource limits), Service, Ingress.
- `deploy/helm/` — full Helm chart v2 with `values.yaml`, `_helpers.tpl`, and templates for all resources (Deployment, Service, ConfigMap, Secret, PVC, Ingress).
- `deploy/terraform/` — Kubernetes Terraform module (`hashicorp/kubernetes` provider): Namespace, Secret, ConfigMap, PVC, Deployment, Service, optional Ingress. Works with any K8s cluster (EKS, GKE, AKS, local).

### 11. Infra Asset Scanning ✅
- **Implemented**: Automatic discovery and indexing of infrastructure and deployment assets beyond documentation files.
- `scanner/scanner.go` — new `SCAN_INFRA_DIRS` env var (default: `deploy/`, `infra/`, `.github/workflows/`) triggers recursive scanning for `.yaml`, `.yml`, `.tf`, `.hcl`, `.toml` files in those directories. Context-aware `classifyFile` assigns specific types: `helm`, `k8s`, `workflow`, `terraform`, `toml`, `infra`.
- Root-level tooling files are scanned by default: `Dockerfile` (`dockerfile`), `docker-compose.yml`/`docker-compose.yaml` (`compose`), `Makefile` (`makefile`), `.mise.toml`/`mise.toml` (`mise`).
- All classified infrastructure files are exposed to AI agents via `list_repos`, `search_docs`, and `get_file_content` — path-traversal protected by the same `IsIndexed()` security model as documentation files.

### 12. Security Input Hardening ✅
- **Implemented**: Input validation and SQL injection prevention across the catalog parser and content search.
- `scanner/parser/catalog.go` — `isValidEntityName` validates entity names from `catalog-info.yaml` against `[a-zA-Z0-9._-]{1,253}` (with optional `namespace/` prefix). Invalid names return a parse error rather than being silently stored.
- `memory/content.go` — SQL LIKE wildcard characters (`%`, `_`, `\`) in search queries are escaped before being interpolated into `LIKE`/`ILIKE` predicates, preventing user-controlled wildcards from scanning the entire table.

### 16. Documentation Site (GitHub Pages) ✅
- **Implemented**: MkDocs Material site auto-deployed to GitHub Pages on every push to `main` that touches `docs/` or `mkdocs.yml`.
- `mkdocs.yml` — Material theme with dark/light mode, tabs, search, Mermaid diagrams, and code copy.
- `docs/index.md` — Landing page with tabbed quick-start, Mermaid architecture diagram, and Material admonitions.
- `docs/tools-reference.md` — Complete reference for all 23 MCP tools with parameters and examples.
- `docs/contributing.md` — Full contributing guide: setup, project structure, testing, PR checklist.
- `docs/roadmap.md` — Public-facing roadmap with completed features and upcoming work.
- `docs/security.md` — Security model and responsible disclosure policy.
- `CONTRIBUTING.md` — Root-level shortcut for GitHub UI, links to full docs.
- `.github/workflows/docs.yml` — Triggers on changes to `docs/**` or `mkdocs.yml`; uses `mkdocs gh-deploy`.
- `docs/requirements.txt` — Pinned `mkdocs-material` version for reproducible builds.

### 14. Graph Traversal & Impact Analysis ✅
- **Implemented**: Server-side BFS traversal so AI agents can answer impact and ownership questions without loading the full graph.
- `memory/traverse.go` — `traverseGraph` performs BFS using SQL `IN` queries per hop (never loads the full graph). Supports `outgoing`, `incoming`, and `both` directions with optional edge-type filtering. Cycle-safe via `visited` map. Observations batch-loaded at the end.
- `memory/memory.go` — `TraverseGraph` exposed on `MemoryService`.
- `tools/ports.go` — `TraverseGraph` added to `GraphStore` interface; `tools/audit.go` — read-only pass-through.
- `tools/traverse_graph.go` — `traverse_graph` MCP tool: `entity` (required), `relation_type` (optional filter), `direction` (outgoing/incoming/both, default outgoing), `depth` (1–10, default 1, clamped). Returns `TraverseGraphResult` with nodes, distances, and paths.
- `tests/traverse_graph/` — 8 E2E scenarios: outgoing depth 1/2, incoming, relation filter, unknown entity, cycle safety, and input validation errors.

### 13. Custom Parser Extension ✅
- **Implemented**: `FileParser` interface and `ParserRegistry` allow custom parsers to be plugged in without forking.
- `scanner/parser/extension.go` — `FileParser` interface, `ParsedFile` struct, `ParsedRelation`, `AuxEntity`, `MergeMode`.
- `scanner/parser/registry.go` — thread-safe `ParserRegistry` with `Register` / `Get` / `All`; panics on duplicate `FileType()`.
- `scanner/scanner.go` — `classifyFile` and `DefaultTargetFiles` driven by registry; backward-compatible.
- `indexer/indexer.go` — single `upsertParsedFile()` replaces 5 duplicate phase methods; iterates registry.
- All built-in parsers implement `FileParser` (gomod, packagejson, pom, catalog, codeowners).
- Integration parsers registered in `main.go`: AsyncAPI, SpringKafka, OpenAPI, Proto.

### 15. Integration Topology Discovery ✅
- **Implemented**: Five new parsers automatically populate producer/consumer and API dependency edges. `get_integration_map` tool returns the complete integration picture of a service in one call.
- `scanner/parser/asyncapi.go` — AsyncAPI 2.x: extracts `publishes_event` / `subscribes_event` edges.
- `scanner/parser/spring_kafka.go` — Spring Kafka `@KafkaListener` / `KafkaTemplate` heuristics.
- `scanner/parser/openapi.go` — OpenAPI/Swagger: `exposes_api` edges.
- `scanner/parser/proto.go` — Protobuf: `provides_grpc` and `depends_on_grpc` edges.
- `memory/integration.go` — `GetIntegrationMap` query with `graph_coverage` (full/partial/inferred/none).
- `tools/get_integration_map.go` — `get_integration_map` MCP tool.
- `tests/integration_map/` — 3 E2E scenarios.

### 29. Relation Confidence Scores ✅
- **Implemented (2026-04-23)**: Added `confidence` field to every relation: `"authoritative"` for relations from explicit contract files, `"inferred"` for heuristic sources, `"ambiguous"` for caller-marked edges.
- `memory/memory.go` — `Relation` struct gains `Confidence string`; `dbRelation` GORM model adds `confidence` column (AutoMigrate; backward-compatible default `"authoritative"`).
- `scanner/parser/extension.go` — `ParsedRelation.Confidence` field; parsers set it: asyncapi/openapi/proto → `"authoritative"`, springkafka/k8sintegration → `"inferred"`.
- `indexer/indexer.go` — passes `Confidence` through; defaults to `"authoritative"` when empty.
- `memory/traverse.go` — new `TraverseEdge` struct; `traverse_graph` result now includes an `edges` array with `from`, `to`, `relationType`, and `confidence`.
- `memory/pathfind.go` — `PathEdge` gains `confidence` field.
- `tools/create_relations.go` — defaults caller-supplied relations to `"authoritative"` when `confidence` is omitted; callers can override by setting `"inferred"` or `"ambiguous"`.
- `tools/traverse_graph.go` — `TraverseGraphResult.Edges []TraverseEdge` added to surface edge confidence on every traversal.

### 2. Semantic Search and Vector Embeddings (RAG) ✅
- **Phase 1 ✅ (2026-04-11)**: Replaced SQL `LIKE` with SQLite FTS5 full-text search — BM25 relevance ranking, Porter stemmer (`authenticate` → `authentication`), multi-word AND queries, special-char-safe query sanitization. Zero new dependencies.
- **Phase 2 ✅ (2026-04-14)**: `semantic_search` MCP tool — vector embeddings via OpenAI or Ollama, cosine similarity ranking, hash-based staleness detection for docs and entities, debounced background re-indexing. Zero-downtime: server starts normally if no embedding provider is configured.

### 27. Benchmark Narrative — "The Number" ✅
- **Implemented**: `benchmark/RESULTS.md` — 99.0% average token savings across 12 canonical questions, F1 1.00 parser accuracy across all 10 parsers. Badge added to README and docs site.

### 25. Graph Visualization Export ✅
- **Implemented**: `memory/export.go` — `ExportGraph(kg, format, title)` renders `html` (self-contained force-directed graph, vanilla JS Canvas, zero dependencies) and `json` (nodes+edges) formats.
- `tools/export_graph.go` — `export_graph` MCP tool: accepts `format`, `title`, and optional `output_path`. Writes to disk or returns content inline.

### 28. Onboarding in 60 Seconds ✅
- **Implemented**: `bin/docscout-init.sh` — curl-installable shell script. Downloads latest binary for the detected OS/arch (falls back to `go run`), writes `.env.local`, and prints the Claude Desktop config snippet. No manual config required for the "try it" path.

---

## Future Work

### 4. Graph Knowledge Access Control (RBAC)
- **Current State**: Any LLM client connected to the MCP server can read any file and entity that was indexed.
- **Goal**: Implement Role-Based Access Control (RBAC) so that sensitive architectural or security documents are only accessible to authorized users or service accounts.

### 5. Multi-Cloud and Platform Adapters
- **Current State**: Hardcoded dependency on GitHub API.
- **Goal**: Build a generic "Provider" interface to support GitLab, Bitbucket, Confluence, Notion, and other internal enterprise wikis out-of-the-box.

**New relation types**:
| Relation | From | To | Source |
|---|---|---|---|
| `publishes_event` | service | event-topic | AsyncAPI, Spring Kafka config |
| `subscribes_event` | service | event-topic | AsyncAPI, Spring Kafka config |
| `provides_grpc` | service | grpc-service | .proto files |
| `depends_on_grpc` | service | grpc-service | .proto imports |
| `exposes_api` | service | api | OpenAPI/Swagger specs |
| `calls_service` | service | service | K8s env vars (`*_SERVICE_HOST`) |

**New entity types**: `event-topic`, `grpc-service` (enriching existing `api`).

**New `get_integration_map` tool**:
```
Input:  service (required), depth (optional, default 1, max 3)
Output: { publishes, subscribes, exposes_api, provides_grpc, grpc_deps, calls, graph_coverage }
        graph_coverage: "full" | "partial" | "inferred" | "none"
```

**Implementation scope**:
- `scanner/parser/asyncapi.go` (new) — AsyncAPI channels → `publishes_event` / `subscribes_event`
- `scanner/parser/springkafka.go` (new) — `application.yml` + `.properties` Kafka config
- `scanner/parser/openapi.go` (new) — OpenAPI/Swagger `info` + `servers` + path count
- `scanner/parser/proto.go` (new) — `.proto` service definitions and imports
- `scanner/parser/k8sintegration.go` (new) — K8s Deployment env vars heuristic
- `scanner/scanner.go` — add `*.proto` to `DefaultTargetFiles`
- `memory/integration.go` (new) — `GetIntegrationMap` aggregation queries
- `memory/memory.go` — expose on `MemoryService`; add to `GraphStore` interface
- `tools/get_integration_map.go` (new) — handler + typed args/result
- `tools/ports.go` — add `GetIntegrationMap` to `GraphStore` interface
- `tools/tools.go` — register tool
- `main.go` — register 5 new parsers in `parser.Default`
- `tests/integration_map/integration_map_test.go` (new) — E2E tests
- `AGENTS.md` — update §7 with new relation types and tool usage

**Requires**: `#13 Custom Parser Extension` (parsers implement `FileParser`).
**Complements**: `#14 Graph Traversal` (traverse_graph works over the new edges once both are implemented).

**Spec**: `docs/superpowers/specs/2026-04-03-integration-topology-discovery-design.md`

### 20. Benchmark Suite

**Goal:** Accuracy (F1 per parser) and token-efficiency benchmarks shipped as `benchmark/RESULTS.md`. Synthetic corpus with committed ground truth. `make benchmark` (no API key) and `make benchmark-live` (Claude API).

### 21. `--benchmark` CLI Mode

**Goal:** Users run `docscout-mcp --benchmark --org myorg` against their own GitHub org and get a shareable markdown report with accuracy F1 and token savings percentages.

### 22. GitHub Actions Action

**Goal:** `docscout-action` — run a DocScout scan in CI and post graph insights as PR comments. Enables teams to see dependency and ownership changes on every PR.

### 23. LLM Eval Harness

**Goal:** Answer-quality evaluation using an LLM judge (beyond token counting). Measures correctness of AI responses, not just cost. Reproducible eval set with expected answers for the canonical question corpus.

### 24. OpenTelemetry Traces

**Goal:** Distributed tracing for production multi-tenant deployments. One span per tool call, per scan, per indexer phase. Compatible with Jaeger, Grafana Tempo, and cloud providers.

### 26. `ingest_url` MCP Tool ✅

- **Implemented (2026-04-23)**: New `ingest_url` tool fetches any public HTTP/HTTPS URL and ingests its content into the knowledge graph.
- `tools/ingest_url.go` — validates URL scheme, checks `ALLOWED_INGEST_DOMAINS` allowlist (empty = allow all), rate-limits to ≤5 req/s per domain (200ms min gap), fetches with 15s timeout and `User-Agent: docscout-mcp/1.0`, extracts `<title>`, `<h1>`–`<h3>` headings, `<meta name="description">`, and word-count estimate from the HTML body. Creates a graph entity (or adds observations to an existing one) and optionally stores raw HTML in the `ContentCache`.
- `tools/tools.go` — `Register` signature extended with `cache *memory.ContentCache`; tool registered inside the `graph != nil && !readOnly` block.
