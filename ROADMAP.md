# DocScout-MCP Roadmap

This document outlines the current technical debts and the path forward for DocScout-MCP to become a more resilient, intelligent, and widely adopted open-source tool.

## Future work

### 1. Incremental Ingestion Pipeline (Event-Driven)
- **Current State**: The scanner uses polling (`SCAN_INTERVAL`) to fetch data from GitHub repositories, which can lead to unnecessary API calls and rate-limiting issues on large organizations.
- **Goal**: Implement GitHub Webhook integrations (Push, Release events) to trigger targeted, real-time scans of only the modified files, saving resources and ensuring the Knowledge Graph is always instantly up to date.

### 2. Semantic Search and Vector Embeddings (RAG)
- **Current State**: Content search relies on exact text matching (`LIKE` queries in SQL).
- **Goal**: Integrate vector embeddings (e.g., using `pgvector` for PostgreSQL or `sqlite-vss`) to allow AI Assistants to perform true semantic searches. This will drastically improve the relevance of the retrieved context.

### 3. Rate Limiting, Resilience, and Circuit Breakers
- **Current State**: Basic per-repository timeouts have been implemented, but massive organizations (500+ repos) can still exhaust the GitHub API rate limit (5000 requests/hour).
- **Goal**: Implement adaptive rate-limiting algorithms with Exponential Backoff and Circuit Breakers to handle massive scale securely without getting blocked by GitHub.

### 4. Graph Knowledge Access Control (RBAC)
- **Current State**: Any LLM client connected to the MCP server can read any file and entity that was indexed.
- **Goal**: Implement Role-Based Access Control (RBAC) so that sensitive architectural or security documents are only accessible to authorized users or service accounts.

### 5. Multi-Cloud and Platform Adapters
- **Current State**: Hardcoded dependency on GitHub API.
- **Goal**: Build a generic "Provider" interface to support GitLab, Bitbucket, Confluence, Notion, and other internal enterprise wikis out-of-the-box.

### 6. Auto-Discovery (AST & Dependencies Parsing)
- **Current State**: The knowledge graph relies heavily on Backstage's `catalog-info.yaml` to understand services and relations.
- **Goal**: Expand the parsers to auto-discover relations without Backstage by analyzing files like `go.mod`, `package.json`, `pom.xml`, and `CODEOWNERS` to infer dependencies and ownership dynamically.

### 7. Deployment and Operations
- **Current State**: Manual deployment via `go run` or raw Docker commands.
- **Goal**: Create production-ready deployment assets ("One-click deploy" Helm charts, Terraform modules, and K8s manifests).

### 8. Observability and Metrics (Prometheus)
- **Current State**: Logs are emitted via `slog`.
- **Goal**: Expose an HTTP metrics endpoint for Prometheus. Instrument the tool usage to measure "which documents are most requested by AIs", helping human teams identify knowledge gaps.

### 9. Knowledge Graph Protection
- **Current State**: AI agents can freely create/delete entities and observations.
- **Goal**: Add strict moderation/guardrails in the prompts to prevent the graph from becoming polluted with LLM hallucinations or accidental widespread deletions.
