# MCP Tools Reference

DocScout-MCP exposes **26 MCP tools** across five categories. All tools are instrumented with call-count metrics and wrapped with panic recovery.

!!! note "Tool availability"
    `search_content` is only registered when `SCAN_CONTENT=true` on a persistent `DATABASE_URL`.
    `ingest_url` requires a non-read-only graph (`GRAPH_READ_ONLY` must not be set).
    All mutation tools (`create_*`, `add_observations`, `delete_*`, `update_entity`, `ingest_url`) are omitted when `GRAPH_READ_ONLY=true`.
    `query_audit_log` and `get_audit_summary` require a persistent `DATABASE_URL`; without it they return a structured error.
    All other tools are always available.

---

## Scanner Tools

### `list_repos`

Lists all repositories in the organization that contain indexed documentation files.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `file_type` | string | | Filter to repos that contain at least one file of this type (e.g. `openapi`, `asyncapi`, `proto`, `helm`, `dockerfile`, `readme`). Leave empty to return all repos. |

**Returns:** Array of `{ name, full_name, description, html_url, files[] }`.

---

### `search_docs`

Searches for documentation files by matching a query term against file paths and repository names.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `query` | string | ✅ | Search term (must not be empty or whitespace-only) |
| `file_type` | string | | Filter to files of this type (e.g. `openapi`, `asyncapi`, `proto`, `readme`, `helm`). Leave empty to return all matching files. |

**Returns:** Array of `{ repo_name, path, type }`.

---

### `get_file_content`

Retrieves the raw content of a specific documentation file from a GitHub repository.

!!! warning "Security"
    Only files that have been indexed by the scanner can be fetched. Arbitrary file paths are rejected, preventing path traversal.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `repo` | string | ✅ | Repository name (e.g. `my-org/my-service`) |
| `path` | string | ✅ | File path as returned by `list_repos` or `search_docs` |

**Returns:** Raw file content as a string.

---

### `get_scan_status`

Returns the current state of the documentation scanner and knowledge graph index. Call this before searching to confirm the index is populated, especially right after startup.

**Parameters:** none

**Returns:**

| Field | Type | Description |
|---|---|---|
| `scanning` | bool | Whether a scan is currently in progress |
| `last_scan_at` | string | RFC3339 timestamp of the last completed scan |
| `repo_count` | int | Number of repos indexed |
| `content_indexed` | int | Files in the content cache (`SCAN_CONTENT=true` only) |
| `graph_entities` | int | Total entities in the knowledge graph |
| `entity_breakdown` | object | Map of `entity_type → count` (e.g. `{"service": 12, "api": 4}`) |
| `content_enabled` | bool | Whether content caching is active |
| `search_mode` | string | `"fts5"` (SQLite FTS5 full-text search), `"like"` (fallback), or `""` (disabled) |
| `read_only` | bool | Whether the server is running in read-only graph mode |

---

### `trigger_scan`

Queues an immediate full repository scan without waiting for the next scheduled interval. The scan runs asynchronously — call `get_scan_status` to monitor progress. Duplicate triggers are coalesced: if a scan is already queued, this is a no-op.

**Parameters:** none

**Returns:**

| Field | Type | Description |
|---|---|---|
| `triggered` | bool | `true` when a scan was newly queued |
| `already_queued` | bool | `true` when a scan was already pending and this request was a no-op |

---

### `search_content`

Full-text search across the content of all cached documentation files. Only available when `SCAN_CONTENT=true`.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `query` | string | ✅ | Term to search inside file content (must not be whitespace-only) |
| `repo` | string | | Filter results to a single repository |

**Returns:** Up to 20 results, each with `{ repo_name, path, snippet }`. The snippet is ~300 characters of context around the first match.

---

## Knowledge Graph Tools

The knowledge graph stores entities (nodes), relations (directed edges), and observations (facts attached to entities).

### `create_entities`

Creates new entities in the knowledge graph. Duplicate names are silently skipped.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `entities` | array | ✅ | Array of `{ name, entityType, observations[] }` |

**Observation rules:** Observations are sanitized before storage — empty strings, strings shorter than 2 characters, strings longer than 500 characters, and duplicates within the batch are rejected. The response includes a `skipped` field listing rejections with reasons.

---

### `create_relations`

Creates directed edges between entities. Duplicate `(from, to, relationType)` triples are silently skipped.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `relations` | array | ✅ | Array of `{ from, to, relationType }` |

---

### `add_observations`

Appends facts to existing entities. Observations that already exist on the entity are skipped (idempotent).

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `observations` | array | ✅ | Array of `{ entityName, contents[] }` |

---

### `read_graph`

Returns the entire knowledge graph. Use `search_nodes`, `open_nodes`, `list_entities`, `list_relations`, or `traverse_graph` when you only need a subset — they are much more token-efficient for large graphs.

**Parameters:** none

**Returns:** `{ entities[], relations[] }`.

---

### `list_entities`

Returns all knowledge graph entities, optionally filtered by type. More efficient than `read_graph` when you only need entities (no relations).

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `entity_type` | string | | Filter to entities of this type (e.g. `service`, `team`, `api`, `event-topic`, `grpc-service`, `person`). Leave empty to return all. |

**Returns:** `{ entities[], relations[] }` — relations are always empty; only entities matching the filter are returned.

---

### `list_relations`

Returns relations from the knowledge graph, filtered by type and/or source entity.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `relation_type` | string | | Filter by relation type (e.g. `depends_on`, `publishes_event`, `subscribes_event`, `exposes_api`, `provides_grpc`, `calls_service`, `owns`, `part_of`). Leave empty for all types. |
| `from_entity` | string | | Filter to relations originating from this entity name. Leave empty for all sources. |

Both filters can be combined. Leave both empty to return all relations.

**Returns:**

| Field | Type | Description |
|---|---|---|
| `relations` | array | Array of `{ from, to, relationType, confidence }` |
| `count` | int | Total number of matching relations |

---

### `ingest_url`

Fetches a public HTTP/HTTPS URL, extracts structured metadata from the HTML (title, headings, meta description, word count), and creates or updates a knowledge graph entity for the page. Optionally stores the raw content in the content cache for `search_content` queries.

!!! note "Availability"
    Only registered when `GRAPH_READ_ONLY` is not set. Domain allowlist can be configured with `ALLOWED_INGEST_DOMAINS` (comma-separated; empty = allow all). Rate-limited to 5 requests/second per domain.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `url` | string | ✅ | Full HTTP or HTTPS URL to fetch. `file://` and `data://` schemes are rejected. |
| `entity_name` | string | | Override the graph entity name. Defaults to the page `<title>` (sanitized). |
| `entity_type` | string | | Entity type to assign. Defaults to `"doc"`. |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `entity_name` | string | The name of the created or updated entity |
| `url` | string | The fetched URL |
| `observation_count` | int | Number of observations written to the graph |
| `cached` | bool | Whether content was stored in the content cache |
| `observations` | array | The observation strings extracted (title, headings, description, word count) |

**Example:**

```
ingest_url(url="https://docs.example.com/api/overview", entity_type="doc")
```

---

### `update_entity`

Renames an entity and/or changes its type. When renaming, all relations and observations that reference the entity are updated atomically — no data is lost.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `name` | string | ✅ | Current name of the entity to update (must match exactly) |
| `new_name` | string | | New name for the entity. All relations and observations are updated atomically. |
| `new_type` | string | | New entity type (e.g. `service`, `team`, `api`). Omit to keep the current type. |

At least one of `new_name` or `new_type` must be provided.

**Returns:**

| Field | Type | Description |
|---|---|---|
| `updated` | bool | `true` when the update succeeded |
| `name` | string | The effective name after the update |

---

### `search_nodes`

Searches entities by name, type, or observation content using a substring match.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `query` | string | ✅ | Substring to match against entity name, type, or observation text |

**Returns:** `{ entities[], relations[] }` — the subgraph of matching entities and the relations between them.

---

### `open_nodes`

Retrieves specific entities by exact name, along with their relations.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `names` | array | ✅ | List of entity names to retrieve |

**Returns:** `{ entities[], relations[] }`.

---

### `traverse_graph`

Performs a BFS traversal from a starting entity, following directed edges up to a configurable depth. Use this instead of `read_graph` for focused impact and dependency queries — it only returns the relevant subgraph.

**Parameters:**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `entity` | string | ✅ | — | Starting entity name |
| `relation_type` | string | | all types | Filter edges by type (e.g. `depends_on`, `owned_by`, `consumes_api`) |
| `direction` | string | | `outgoing` | `outgoing` · `incoming` · `both` |
| `depth` | int | | `1` | Number of hops to follow (1–10, clamped) |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `start_entity` | string | The starting entity name |
| `nodes` | array | Reached nodes, each with `name`, `entityType`, `observations`, `distance`, `path` |
| `edges` | array | Traversed edges, each with `from`, `to`, `relationType`, `confidence` |
| `total_found` | int | Number of reachable nodes |

The `confidence` field on each edge is `"authoritative"`, `"inferred"`, or `"ambiguous"` — see [Relation Confidence Scores](benchmarks.md#relation-confidence-scores).

**Example queries:**

```
# Impact: who depends on payment-api?
traverse_graph(entity="payment-api", relation_type="depends_on", direction="incoming", depth=1)

# Transitive deps: full dependency tree of auth-service
traverse_graph(entity="auth-service", relation_type="depends_on", direction="outgoing", depth=5)

# Ownership: who owns checkout-service and its direct dependencies?
traverse_graph(entity="checkout-service", direction="both", depth=2)
```

---

### `get_integration_map`

Returns the complete integration topology of a service in a single call: which events it publishes and subscribes to, which APIs and gRPC services it exposes or depends on, and which services it calls directly. Each entry includes a `confidence` level so the AI agent can distinguish authoritative contract declarations (AsyncAPI, proto, OpenAPI) from inferred config values (Spring Kafka, K8s env vars).

**Parameters:**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `service` | string | ✅ | — | Entity name of the service in the knowledge graph |
| `depth` | int | | `1` | Number of integration hops to include (1–3) |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `service` | string | The queried service name |
| `publishes` | array | Event topics this service publishes to |
| `subscribes` | array | Event topics this service subscribes from |
| `exposes_api` | array | APIs this service exposes |
| `provides_grpc` | array | gRPC services this service provides |
| `grpc_deps` | array | gRPC services this service depends on |
| `calls` | array | Services this service calls directly |
| `graph_coverage` | string | `full` · `partial` · `inferred` · `none` |

Each edge includes `target`, `confidence`, and optionally `schema`, `version`, `paths`, `source_repo`.

**`graph_coverage` values:**

| Value | Meaning |
|---|---|
| `full` | At least one authoritative source (AsyncAPI, proto, OpenAPI), no inferred sources |
| `partial` | Mix of authoritative and inferred sources |
| `inferred` | All relations come from config heuristics (Spring Kafka, K8s env vars) |
| `none` | No integration data found for this service |

---

### `find_path`

Finds the shortest connection path between two entities using undirected BFS. Returns the ordered sequence of directed edges (from, relationType, to) connecting them, regardless of edge direction.

**Parameters:**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `from` | string | ✅ | — | Starting entity name |
| `to` | string | ✅ | — | Destination entity name |
| `max_depth` | int | | `6` | Maximum number of hops to search (1–10) |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `found` | bool | Whether a path was found within `max_depth` |
| `path` | array | Ordered sequence of `{ from, relation_type, to, confidence }` edges |
| `hops` | int | Number of edges in the path |

**Example queries:**

```
# Is there any dependency chain between payment-svc and auth-svc?
find_path(from="payment-svc", to="auth-svc")

# What connects team-platform to checkout-service?
find_path(from="team-platform", to="checkout-service", max_depth=4)
```

---

### `delete_entities`

Removes entities and all their associated relations and observations.

!!! warning "Mass-delete guard"
    Deleting more than 10 entities in a single call is rejected unless `confirm: true` is explicitly set.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `entityNames` | array | ✅ | Names of entities to delete |
| `confirm` | bool | | Required when deleting more than 10 entities |

---

### `delete_observations`

Removes specific observations from entities without deleting the entities themselves.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `observations` | array | ✅ | Array of `{ entityName, observations[] }` — the observation strings to remove |

---

### `delete_relations`

Removes specific directed edges from the graph.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `relations` | array | ✅ | Array of `{ from, to, relationType }` triples to remove |

---

## Observability Tools

### `get_usage_stats`

Returns per-tool call counts and the top 20 most-fetched documents since server start. Useful for identifying which documentation areas are most frequently accessed by AI agents, helping teams spot knowledge gaps.

**Parameters:** none

**Returns:**

| Field | Type | Description |
|---|---|---|
| `tool_calls` | object | Map of `tool_name → call_count` |
| `top_documents` | array | Top 20 docs by access count: `{ repo, path, count }` |

The `/metrics` HTTP endpoint (when `HTTP_ADDR` is set) emits the same data as Prometheus counters:

- `docscout_tool_calls_total{tool}` — total calls per tool
- `docscout_document_accesses_total{repo,path}` — total fetches per document

---

## Audit Tools

!!! info "Requires persistent storage"
    Both tools require `DATABASE_URL` to point to a persistent store (SQLite file or PostgreSQL). Without it, they return:
    `"audit persistence not enabled — set DATABASE_URL to a persistent store"`.

### `query_audit_log`

Retrieves raw audit events for every graph mutation, optionally filtered. Useful for governance reviews, incident investigation, and understanding what an AI agent changed and when.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `agent` | string | | Filter by agent identity (e.g. `"indexer-bot"`, `"claude-desktop"`) |
| `tool` | string | | Filter by MCP tool name (e.g. `"create_entities"`, `"delete_entities"`) |
| `operation` | string | | Filter by operation type: `create`, `delete`, `update`, or `add` |
| `outcome` | string | | Filter by outcome: `ok` or `error` |
| `since` | string | | RFC3339 timestamp lower bound (e.g. `"2026-04-20T00:00:00Z"`) |
| `limit` | int | | Max events to return. Default `50`, max `500` |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `events` | array | List of `AuditEvent` objects (see below) |
| `total` | int | Total matching rows (before limit) |

**`AuditEvent` fields:**

| Field | Type | Description |
|---|---|---|
| `id` | string | UUIDv7 primary key — lexicographic sort = chronological order |
| `created_at` | string | RFC3339 timestamp |
| `agent` | string | Resolved agent identity |
| `tool` | string | MCP tool name |
| `operation` | string | `create` \| `delete` \| `update` \| `add` |
| `targets` | string | JSON array of affected entity/relation names |
| `count` | int | Number of items mutated |
| `outcome` | string | `ok` \| `error` |
| `error_msg` | string | Error description (populated on failure) |

**HTTP equivalent:**

```
GET /audit?agent=indexer-bot&operation=delete&since=2026-04-20T00:00:00Z&limit=100
→ 200 JSON: { "events": [...], "total": N }
→ 503 JSON: { "error": "audit persistence not enabled..." }
```

---

### `get_audit_summary`

Returns an anomaly-focused report over a rolling time window. Designed for governance dashboards and detecting unexpected or risky agent behaviour.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `window` | string | | Rolling time window: `1h`, `24h` (default), or `7d` |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `total_mutations` | int | Total graph mutations in the window |
| `by_agent` | object | Map of `agent → mutation count` |
| `by_operation` | object | Map of `operation → mutation count` |
| `error_rate` | float | Fraction of mutations that resulted in an error |
| `risky_events` | array | Events that matched at least one risky criterion (see below) |

**Risky event criteria:**

- Any `delete` operation with `count > 10` (mass deletion)
- Any event where `agent = "unknown"` (unidentified agent)
- Any agent that produced more than 5 errors within a 1-hour window (error burst)

**HTTP equivalent:**

```
GET /audit/summary?window=24h
→ 200 JSON: { "total_mutations": N, "by_agent": {...}, "by_operation": {...}, "error_rate": 0.02, "risky_events": [...] }
→ 503 JSON: { "error": "audit persistence not enabled..." }
```

---

## MCP Discovery Tools

### `discover_mcp_servers`

Discovers and catalogs MCP servers found in indexed GitHub repositories. Supports three query modes:

1. **Inventory** — list all known MCP servers across the org
2. **Capability search** — find servers that expose a specific tool (`tool_name` filter, case-insensitive substring match)
3. **Dependency lookup** — combine with `traverse_graph` on a service entity to follow `uses_mcp` edges

Only returns servers discovered from indexed config files (`.mcp.json`, `claude_desktop_config.json`, `.cursor/mcp.json`, `mcp.json`, `.vscode/mcp.json`). No live connections to MCP servers are made.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `repo` | string | | Filter results to a specific repository name |
| `tool_name` | string | | Return only servers that have a tool matching this name (case-insensitive substring) |
| `transport` | string | | Filter by transport type: `stdio`, `http`, or `sse` |
| `limit` | int | | Max servers to return. Default `20`, max `100` |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `servers` | array | List of `MCPServerResult` objects (see below) |
| `total` | int | Number of servers returned (after filtering) |

**`MCPServerResult` fields:**

| Field | Type | Description |
|---|---|---|
| `name` | string | Server name as declared in the config file |
| `repo` | string | Repository where the config was discovered |
| `transport` | string | `stdio`, `http`, `sse`, or `unknown` |
| `command` | string | Full command string (stdio servers only) |
| `url` | string | Endpoint URL (http/sse servers only) |
| `tools` | array | Tool names available on this server (from registry or inline config) |
| `config_file` | string | Config filename where this server was found |

**Example queries:**

```
# Which MCP servers does my org use?
discover_mcp_servers()

# Which servers can search the web?
discover_mcp_servers(tool_name="search")

# Which MCP servers does payment-service use?
traverse_graph(entity="payment-service", relation_type="uses_mcp", direction="outgoing", depth=1)
```

**Well-known server enrichment:**

The following servers are automatically enriched with tool observations even if not declared inline: `github`, `filesystem`, `postgres`, `fetch`, `brave-search`, `slack`. Unknown servers are indexed with transport/command/url only.

---

## Visualization Tools

### `export_graph`

Exports the full knowledge graph as a self-contained interactive HTML visualization or a raw JSON nodes+edges payload.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `format` | string | | `html` (default) — self-contained force-directed graph with vanilla JS Canvas; `json` — nodes+edges array |
| `title` | string | | Title shown in the exported artifact. Defaults to `"Knowledge Graph"` |
| `output_path` | string | | Absolute path to write the file (e.g. `/tmp/graph.html`). If omitted, content is returned inline |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `format` | string | `html` or `json` |
| `entity_count` | int | Total entities in the exported graph |
| `edge_count` | int | Total relations in the exported graph |
| `output_path` | string | Path written to (only when `output_path` was provided) |
| `content` | string | Inline file content (only when `output_path` was omitted) |

**Example queries:**

```
# Get an interactive visualization of the full graph
export_graph(format="html", title="My Org")

# Write the graph to disk and open it in a browser
export_graph(format="html", output_path="/tmp/graph.html")

# Get the graph as JSON for downstream processing
export_graph(format="json")
```

---

## Semantic Search Tools

### `semantic_search`

Performs vector-embedding-based semantic search across indexed documentation and knowledge graph entities. Returns cosine-similarity-ranked results. Requires `DOCSCOUT_EMBED_OPENAI_KEY` or `DOCSCOUT_EMBED_OLLAMA_URL` to be configured; returns an error otherwise.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `query` | string | ✓ | Natural-language search query |
| `target` | string | | What to search: `content` (indexed docs), `entities` (knowledge graph), or `both` (default) |
| `top_k` | int | | Maximum results per target. Default `5`, max `20` |
| `repo` | string | | Scope content search to a single repository full name (e.g. `org/payment-service`) |

**Returns:**

| Field | Type | Description |
|---|---|---|
| `content_results` | array | Matched document snippets with similarity score, repo, and path |
| `entity_results` | array | Matched graph entities with similarity score and observations |
| `stale_docs` | int | Number of docs whose embeddings are outdated (background re-index pending) |
| `stale_entities` | int | Number of entities whose embeddings are outdated |

**Example queries:**

```
# Find docs related to authentication
semantic_search(query="authentication and JWT token validation")

# Find services related to payment processing
semantic_search(query="payment processing and fraud detection", target="entities")

# Scope to a specific repo
semantic_search(query="database migrations", target="content", repo="org/backend-service")
```
