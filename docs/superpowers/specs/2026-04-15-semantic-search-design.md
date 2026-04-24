# Semantic Search ("Plus" Feature) — Design Spec

## Goal

Add opt-in vector-based semantic search to DocScout-MCP, enabling natural-language queries over both indexed documentation content and knowledge graph entities. The feature is disabled by default and activates only when `SEMANTIC_PROVIDER` is set.

## Architecture

A pluggable `EmbeddingProvider` interface gates all vector work. When no provider is configured, the `semantic_search` tool returns a clear "not enabled" error and the rest of the server runs normally — no startup penalty, no required dependencies.

```
┌─────────────────────────────────────────────────────┐
│                    MCP Tool Layer                    │
│              semantic_search (unified)               │
└──────────────┬──────────────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────┐
│                  SemanticSearcher                    │
│   SearchDocs(ctx, query, topK, repo) []DocResult     │
│   SearchEntities(ctx, query, topK) []EntityResult    │
└──────────────┬──────────────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────┐
│           EmbeddingProvider (interface)              │
│   Embed(ctx, texts []string) ([][]float32, error)    │
├─────────────────────┬───────────────────────────────┤
│  OpenAIProvider     │  OllamaProvider               │
│  (text-embedding-3) │  (nomic-embed-text)           │
└─────────────────────┴───────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────┐
│           VectorStore (in existing DB)               │
│  doc_embeddings(doc_id, vector BLOB, model)          │
│  entity_embeddings(entity_name, vector BLOB, model)  │
│  cosine similarity computed in Go                    │
└─────────────────────────────────────────────────────┘
```

## File Structure

| File | Responsibility |
|---|---|
| `embeddings/provider.go` | `EmbeddingProvider` interface + `NewProvider(cfg)` factory |
| `embeddings/openai.go` | OpenAI embeddings REST client |
| `embeddings/ollama.go` | Ollama embeddings REST client |
| `embeddings/similarity.go` | In-process cosine similarity, top-K selection |
| `embeddings/store.go` | `VectorStore`: DB schema migration, upsert, vector scan |
| `embeddings/indexer.go` | `Indexer`: triggered after scans, batch-embeds new/changed docs and entities |
| `embeddings/searcher.go` | `SemanticSearcher`: wires provider + store, exposes `SearchDocs` / `SearchEntities` |
| `tools/semantic_search.go` | MCP tool definition and handler |
| `main.go` | Initialise provider, VectorStore, Indexer, SemanticSearcher; wire into server |

No existing files are modified except `main.go` (wiring) and the DB auto-migration block.

## MCP Tool

### `semantic_search`

**Description:** Runs a natural-language semantic search over indexed documentation content and/or knowledge graph entities using vector embeddings. Requires the server to be started with `SEMANTIC_PROVIDER` set to `"openai"` or `"ollama"`.

**Arguments:**

| Argument | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Natural-language search query |
| `target` | string | no | `"content"`, `"entities"`, or `"both"` (default: `"both"`) |
| `top_k` | int | no | Number of results per target (default: 5, max: 20) |
| `repo` | string | no | Scope content search to a single repository (full name, e.g. `org/service`) |

**Returns:**

```json
{
  "content_results": [
    {
      "doc_id": "org/service#docs/api.md",
      "repo": "org/service",
      "path": "docs/api.md",
      "score": 0.91,
      "snippet": "first 300 chars of the document"
    }
  ],
  "entity_results": [
    {
      "name": "payment-service",
      "entity_type": "service",
      "score": 0.87,
      "observations": ["lang:go", "owner:platform-team"]
    }
  ]
}
```

When `target` is `"content"`, `entity_results` is omitted. When `target` is `"entities"`, `content_results` is omitted.

## EmbeddingProvider Interface

```go
// EmbeddingProvider generates vector embeddings for a batch of texts.
// Implementations must be safe for concurrent use.
type EmbeddingProvider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    // ModelKey returns a stable string identifying the provider and model,
    // formatted as "<provider>:<model>" (e.g. "openai:text-embedding-3-small",
    // "ollama:nomic-embed-text"). A change in ModelKey triggers re-indexing
    // of all stored vectors.
    ModelKey() string
}
```

### OpenAIProvider

- Calls `POST https://api.openai.com/v1/embeddings`
- Model: configurable via `OPENAI_EMBEDDING_MODEL`, default `text-embedding-3-small`
- Batches up to 2048 texts per API call
- Returns `ErrRateLimit` on HTTP 429 (caller may retry)

### OllamaProvider

- Calls `POST <OLLAMA_BASE_URL>/api/embed`
- Model: configurable via `OLLAMA_EMBEDDING_MODEL`, default `nomic-embed-text`
- Embeds one text at a time (Ollama does not batch)
- Returns descriptive error when Ollama is unreachable

## VectorStore

Two tables added to the existing database via auto-migration:

```sql
CREATE TABLE IF NOT EXISTS doc_embeddings (
    doc_id       TEXT PRIMARY KEY,
    content_hash TEXT NOT NULL,     -- SHA-256 of the raw doc text at index time.
                                    --   Recomputed on each scan; row is re-embedded
                                    --   when this hash differs from the stored value.
                                    --   Prevents the vector from silently drifting out
                                    --   of sync as documentation evolves over time.
    vector       BLOB NOT NULL,     -- []float32 encoded as little-endian IEEE 754.
                                    --   Valid only while content_hash matches the
                                    --   current document and model_key matches the
                                    --   active provider. Treat as stale otherwise.
    model_key    TEXT NOT NULL,     -- Provider + model identifier (e.g. "openai:text-embedding-3-small").
                                    --   A change here triggers full re-indexing for this repo.
    updated_at   DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS entity_embeddings (
    entity_name TEXT PRIMARY KEY,
    obs_hash    TEXT NOT NULL,      -- SHA-256 of sorted observations joined with "\n".
                                    --   Recomputed after create_entities / add_observations /
                                    --   delete_entities; row is re-embedded when this hash
                                    --   differs, keeping the vector in sync with the live
                                    --   knowledge graph as entities accumulate observations.
    vector      BLOB NOT NULL,      -- []float32 encoded as little-endian IEEE 754.
                                    --   Valid only while obs_hash matches current observations
                                    --   and model_key matches the active provider.
    model_key   TEXT NOT NULL,
    updated_at  DATETIME NOT NULL
);
```

Vector encoding: `[]float32` serialised as little-endian IEEE 754 bytes (4 bytes per dimension). Decoded in Go before similarity computation.

**Query path**: at search time, load all `doc_embeddings` rows whose `model_key` matches `provider.ModelKey()`, then filter in Go to those whose `content_hash` still matches the hash of the current document content (re-read from the `docs` table). For entities, the same: filter to rows whose `obs_hash` matches `SHA-256(current_sorted_observations)`. Rows that fail either check are excluded from results. The `semantic_search` response includes `stale_docs` and `stale_entities` counts so the caller knows how many items are pending re-indexing.

Full table scan is acceptable: typical org sizes are well under 10,000 documents.

## Indexer

`Indexer` is triggered in two ways:
1. **Post-scan**: after `trigger_scan` / background scan completes for a repo, `IndexDocs(repoFullName)` is called.
2. **Post-mutation**: after `create_entities`, `add_observations`, or `delete_entities`, the affected entity names are queued for `IndexEntities(names...)` with a 2-second debounce to collapse rapid bursts.

**Staleness detection (docs):** `IndexDocs` computes `SHA-256(content)` for every current doc in the repo and compares it against `doc_embeddings.content_hash`. Rows where the hash or `model_key` differs are re-embedded. New docs are embedded. Docs no longer present in the repo have their `doc_embeddings` row deleted.

**Staleness detection (entities):** `IndexEntities` fetches current observations for each name, computes `SHA-256(sorted_obs_joined)`, and compares against `entity_embeddings.obs_hash`. Re-embeds only if the hash or `model_key` differs.

- Embeds in batches of 50 (OpenAI-friendly); Ollama is called sequentially.
- Upserts vectors. A single-doc/entity failure does not abort the batch — errors are logged to stderr and the indexer continues.
- Runs in a background goroutine; a mutex serialises concurrent indexing runs.

## Configuration

Provider selection is implicit: set the relevant key/URL to enable a provider. If both are set, OpenAI takes precedence and a warning is logged to stderr. Neither set → feature disabled.

| Env Var | Default | Description |
|---|---|---|
| `DOCSCOUT_EMBED_OPENAI_KEY` | _(unset)_ | OpenAI API key. Setting this enables the OpenAI provider. |
| `DOCSCOUT_EMBED_OPENAI_MODEL` | `text-embedding-3-small` | OpenAI embedding model override |
| `DOCSCOUT_EMBED_OLLAMA_URL` | _(unset)_ | Ollama base URL (e.g. `http://localhost:11434`). Setting this enables the Ollama provider. |
| `DOCSCOUT_EMBED_OLLAMA_MODEL` | `nomic-embed-text` | Ollama embedding model override |

## Graceful Degradation

- Neither `DOCSCOUT_EMBED_OPENAI_KEY` nor `DOCSCOUT_EMBED_OLLAMA_URL` is set → `semantic_search` returns structured error: `"semantic search not enabled: set DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL"`. Server starts normally.
- Ollama unreachable → tool call returns descriptive error. Existing keyword search (`search_docs`) is unaffected.
- OpenAI rate limit → tool call returns descriptive error. Indexing will retry on next scan.
- Model key changes → stale vectors are ignored at query time; re-indexing runs automatically on next scan.

## Out of Scope

- No pgvector dependency; all cosine similarity computed in Go.
- No hybrid keyword+semantic blending in this phase.
- No per-user or per-team RBAC filtering (separate future item).
- No streaming responses.
