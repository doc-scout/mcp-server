# MCP Tools Reference

DocScout-MCP exposes **16 MCP tools** across three categories. All tools are instrumented with call-count metrics and wrapped with panic recovery.

!!! note "Tool availability"
    `search_content` is only registered when `SCAN_CONTENT=true` on a persistent `DATABASE_URL`.
    All other tools are always available.

---

## Scanner Tools

### `list_repos`

Lists all repositories in the organization that contain indexed files.

**Parameters:** none

**Returns:** Array of `{ name, full_name, description, html_url, files[] }`.

---

### `search_docs`

Searches for documentation files by matching a query term against file paths and repository names.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `query` | string | ✅ | Search term (must not be empty or whitespace-only) |

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
| `content_enabled` | bool | Whether content caching is active |

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

Returns the entire knowledge graph. Use `search_nodes`, `open_nodes`, or `traverse_graph` when you only need a subset — they are much more token-efficient for large graphs.

**Parameters:** none

**Returns:** `{ entities[], relations[] }`.

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
| `total_found` | int | Number of reachable nodes |

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
