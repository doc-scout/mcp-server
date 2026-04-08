# Roadmap

This page tracks completed features and the path forward for DocScout-MCP.

For the full technical roadmap with implementation details, see [`ROADMAP.md`](https://github.com/leonancarvalho/docscout-mcp/blob/main/ROADMAP.md) in the repository.

---

## Completed

| # | Feature | Summary |
|---|---|---|
| 1 | **Incremental Ingestion (Webhooks)** | GitHub Webhooks trigger instant single-repo scans on push/create/delete events, bypassing the polling cycle. |
| 3 | **Rate Limiting & Resilience** | Every GitHub API call is wrapped with up to 3 retries: primary rate limit wait, abuse rate limit `Retry-After`, and exponential backoff for transient 5xx errors. |
| 6 | **Auto-Discovery (AST & Dependencies)** | Automatic service entity and dependency graph inference from `go.mod`, `package.json`, `pom.xml`, and `CODEOWNERS` — no Backstage catalog-info required. |
| 7 | **Deployment & Operations** | Multi-arch Docker image, docker-compose profiles, Kubernetes manifests, Helm chart v2, and Terraform module. |
| 8 | **Observability & Metrics** | Per-tool call counters and per-document access tracking via `get_usage_stats` tool and `/metrics` Prometheus endpoint. |
| 9 | **Knowledge Graph Protection** | Mass-delete guard (> 10 requires `confirm: true`), observation quality filter, and full mutation audit log to slog. |
| 10 | **Architecture Discovery & Content Search** | Auto-populates the knowledge graph from `catalog-info.yaml` (Backstage format). Opt-in full-text content search via `search_content`. |
| 11 | **Infra Asset Scanning** | Recursive scanning of `deploy/`, `infra/`, `.github/workflows/` for Helm, Terraform, K8s, and workflow files. |
| 12 | **Security Input Hardening** | Entity name validation, SQL LIKE wildcard escaping, whitespace-only query rejection, constant-time bearer token comparison, HTTP server timeouts. |
| 14 | **Graph Traversal & Impact Analysis** | `traverse_graph` tool: server-side BFS with configurable direction, edge-type filter, and depth. Answers impact and ownership questions without loading the full graph. |
| 16 | **Documentation Site (GitHub Pages)** | MkDocs Material site auto-deployed to GitHub Pages on every push to `main`. |

---

## Future Work

### Semantic Search & Vector Embeddings (RAG)

**Current state:** Content search uses SQL `LIKE` queries — exact substring matching only.

**Goal:** Integrate vector embeddings (`pgvector` for PostgreSQL or `sqlite-vss`) so AI agents can perform semantic searches and find relevant docs even without exact keyword matches.

---

### Custom Parser Extension (#13)

**Current state:** Adding a new manifest parser requires edits in 4+ files. Every parser is hardcoded in the indexer.

**Goal:** A `FileParser` interface and `ParserRegistry` so users can plug in custom formats (e.g. `Pipfile`, `.tool-versions`, `chart.lock`) without forking.

```go
type FileParser interface {
    FileType()  string
    Filenames() []string
    Parse([]byte) (ParsedFile, error)
}
```

Registration in `main.go`:
```go
import _ "mypkg/pipfile" // triggers init(), zero other changes required
```

---

### Integration Topology Discovery (#15)

**Current state:** The graph has `depends_on` edges from package manifests, but no understanding of runtime integrations — Kafka topics, gRPC services, HTTP APIs.

**Goal:** Five new parsers (AsyncAPI, Spring Kafka, OpenAPI, Proto, K8s env vars) automatically populate producer/consumer and API dependency edges. A `get_integration_map` tool returns the complete integration picture of a service in a single call, including a `graph_coverage` field so the AI knows how much to trust the answer.

**Requires:** Custom Parser Extension (#13).

---

### Graph Knowledge Access Control (RBAC)

**Goal:** Role-Based Access Control so sensitive architectural or security documents are only accessible to authorized users or service accounts.

---

### Multi-Cloud and Platform Adapters

**Goal:** A generic Provider interface to support GitLab, Bitbucket, Confluence, Notion, and enterprise wikis alongside GitHub.
