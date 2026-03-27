# Architecture Discovery — Design Spec

**Date:** 2026-03-26
**Status:** Approved
**Goal:** Enable AI agents to understand distributed system architecture across a company's GitHub organization — answering questions like "which services consume this event?", "what depends on this service?", and "which service is responsible for payment transactions?"

---

## Problem

DocScout-MCP today indexes documentation file *paths* and exposes raw content on demand. It does not understand the *relationships* between services. The knowledge graph exists but must be populated entirely by hand through AI tool calls. Two gaps prevent architecture discovery:

1. **No content search** — `search_docs` matches only file paths and repo names, not what docs say.
2. **No automatic graph population** — structured dependency data already present in `catalog-info.yaml` files is ignored at scan time.

---

## Solution: Approach C — Auto-Graph + Content Search

Two complementary layers:

- **Phase 1 (Auto-Graph):** During each scan, parse `catalog-info.yaml` files and automatically populate the knowledge graph with services, teams, systems, and their relationships.
- **Phase 2 (Content Search):** Cache raw file content in the DB (opt-in), enabling full-text search across all indexed documentation.

Both phases keep the scanner and memory packages fully decoupled — a new `indexer/` package bridges them.

---

## Architecture

### Data Flow

```
GitHub → Scanner (periodic)
              ├─→ Content Cache (DB)    ←── search_content tool
              └─→ Catalog Parser
                        └─→ Auto-Indexer → Knowledge Graph ←── existing graph tools
```

### New Components

| Component | Package | Responsibility |
|---|---|---|
| `CatalogParser` | `scanner/parser/catalog.go` | Parse `catalog-info.yaml` → typed `ParsedCatalog` struct |
| `ContentCache` | `memory/content.go` | `doc_contents` table; SHA-based incremental fetch |
| `AutoIndexer` | `indexer/indexer.go` | Upsert graph from parsed catalogs; soft-delete stale entities |
| `search_content` tool | `tools/tools.go` | Full-text search over cached content |
| `get_scan_status` tool | `tools/tools.go` | Expose `Scanner.Status()` + content/graph counts |

The scanner gets an optional `OnScanComplete func(repos []RepoInfo)` callback. `main.go` wires the `AutoIndexer` into this callback. Scanner and memory packages remain decoupled.

---

## Component Design

### 1. Content Cache (`memory/content.go`)

New GORM model added alongside existing models:

```go
type dbDocContent struct {
    ID        uint      `gorm:"primaryKey;autoIncrement"`
    RepoName  string    `gorm:"index;uniqueIndex:idx_repo_path"`
    Path      string    `gorm:"uniqueIndex:idx_repo_path"`
    SHA       string
    Content   string    `gorm:"type:text"`
    IndexedAt time.Time
}
```

**Rules:**
- Only active when `SCAN_CONTENT=true`. Silently disabled (with a stderr warning) when using in-memory SQLite — content would be lost on restart and risks OOM.
- Files larger than `MAX_CONTENT_SIZE` (default 200 KB) are stored as a truncation marker string and skipped for search.
- SHA-based diffing: if `sha` matches the stored value, skip re-fetch. After the first scan, only changed files cost API calls.
- Content is stored per-repo. When a repo is removed from the scan results, its content rows are deleted.

### 2. Catalog Parser (`scanner/parser/catalog.go`)

Pure function — no DB, no network. Takes raw YAML bytes, returns `ParsedCatalog`. Easy to unit test in isolation.

**Fields extracted from Backstage format:**

```yaml
metadata.name        → entity name
spec.type            → entityType (service, api, library, website, ...)
spec.lifecycle       → observation: "lifecycle:production"
spec.owner           → Relation(service → team, "owned_by")
spec.system          → Relation(service → system, "part_of")
spec.dependsOn[]     → Relation(service → X, "depends_on")
spec.providesApis[]  → Relation(service → api, "provides_api")
spec.consumesApis[]  → Relation(service → api, "consumes_api")
metadata.description → observation: "description:..."
metadata.tags[]      → observation: "tag:payment", "tag:critical"
```

**Backstage `kind` mapping:**

| kind | entityType |
|---|---|
| Component | `spec.type` (service, library, website…) |
| API | "api" |
| System | "system" |
| Resource | "resource" |
| Group | "team" |
| Unknown | "component" |

**Resilience:** missing optional fields are skipped silently. Malformed YAML logs a warning to stderr and skips the file — never panics or aborts the scan.

OpenAPI/Swagger parsing is **explicitly out of scope** for this iteration (spec version fragmentation: 2.0 vs 3.0 vs 3.1 is non-trivial). Revisit after catalog-info.yaml is stable.

### 3. Auto-Indexer (`indexer/indexer.go`)

Runs at the end of each scan cycle via the `OnScanComplete` callback.

**Upsert strategy:**

```
For each ParsedCatalog entity:
  1. Entity does not exist → create with auto observations
  2. Entity exists, tagged _source:catalog-info → update auto observations, preserve manual ones
  3. Entity exists, NOT tagged _source:catalog-info → add missing observations only, never overwrite
```

Every auto-generated entity gets two fixed observations:
- `_source:catalog-info` — marks it as auto-generated, used for upsert decisions
- `_scan_repo:owner/repo` — links back to the source repo for soft-delete tracking

All auto-generated relations are idempotent — existing `createRelations` deduplicates by `(from, to, relationType)`.

**Soft-delete on rescan:**

After every full scan, the indexer finds all entities tagged `_scan_repo:X` where repo `X` is no longer in the current scan results. It adds observation `_status:archived` to those entities. No hard deletes — history is preserved. AI agents see the `_status:archived` observation and treat data as stale.

### 4. New MCP Tools

#### `search_content`

```
Description: Full-text search across all cached documentation content.
             Only available when SCAN_CONTENT=true.

Input:
  query  string  (required) — term to search inside file content
  repo   string  (optional) — filter results to a single repository

Output:
  []{ repo_name, path, snippet }
  snippet: ~300 chars of context around the first match
  max 20 results
```

Backend: SQL `LIKE` for SQLite, `ILIKE` for PostgreSQL. Returns a clear, actionable error message if content caching is disabled (not a silent empty result).

#### `get_scan_status`

```
Description: Returns the current state of the documentation scanner and index.
             Use this before searching to confirm the index is populated.

Input: none

Output:
  scanning:         bool
  last_scan_at:     RFC3339 timestamp
  repo_count:       int
  content_indexed:  int  (files in doc_contents table)
  graph_entities:   int  (entities in knowledge graph)
  content_enabled:  bool (reflects SCAN_CONTENT env var)
```

Exposes the already-implemented `Scanner.Status()` method that is currently unreachable by AI agents.

---

## Configuration

Two new environment variables:

| Variable | Default | Description |
|---|---|---|
| `SCAN_CONTENT` | `false` | Enable content caching and `search_content` tool |
| `MAX_CONTENT_SIZE` | `204800` | Max bytes per file to cache (200 KB). Larger files skipped. |

Existing `DATABASE_URL` controls storage for both content cache and knowledge graph.

---

## Security Considerations

- Content cache inherits the existing `IsIndexed()` security model: only files whitelisted during scan are stored or searchable. The LLM cannot trigger fetching of arbitrary paths.
- `search_content` snippet extraction is done server-side. Raw content is never returned in bulk — only matched snippets.
- Auto-indexer writes to the knowledge graph under a controlled namespace (`_source:catalog-info`). It cannot overwrite entities the AI created manually (see upsert strategy above).

---

## Testing Plan

- **Unit:** `CatalogParser` tested with fixture YAML files covering all Backstage kinds, missing fields, malformed YAML.
- **Unit:** `ContentCache` upsert logic tested with SHA change detection and size cap.
- **Unit:** `AutoIndexer` upsert and soft-delete logic tested with mock store.
- **Integration:** Extend existing `integration_test.go` — add `search_content` and `get_scan_status` to the E2E tool verification. Add `TestE2E_SearchContent` and `TestE2E_ScanStatus`.

---

## Out of Scope

- OpenAPI/Swagger parsing (revisit as Phase 3)
- Semantic / embedding-based search
- HTTP transport authentication
- Multi-org support
- Webhook-based scan triggers
