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
    // ModelKey returns a stable string identifying the model in use.
    // A change in ModelKey triggers re-indexing of stored vectors.
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
    doc_id    TEXT PRIMARY KEY,
    vector    BLOB NOT NULL,        -- []float32 encoded as little-endian IEEE 754
    model_key TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS entity_embeddings (
    entity_name TEXT PRIMARY KEY,
    vector      BLOB NOT NULL,
    model_key   TEXT NOT NULL,
    updated_at  DATETIME NOT NULL
);
```

Vector encoding: `[]float32` serialised as little-endian IEEE 754 bytes (4 bytes per dimension). Decoded in Go before similarity computation.

**Query path**: load all rows whose `model_key` matches the active provider, compute cosine similarity in Go, return top-K.

Full table scan is acceptable: typical org sizes are well under 10,000 documents.

## Indexer

`Indexer` is called by the scanner after each scan completes (via a post-scan hook in `main.go`).

- Fetches all docs/entities whose `updated_at` is newer than the stored vector's `updated_at`, or whose stored `model_key` differs from the active provider's `ModelKey()`.
- Embeds in batches of 100 (configurable constant).
- Upserts vectors. A scan failure does not abort — the indexer logs errors and continues.
- Runs in a background goroutine; concurrent scans are serialised with a mutex.

## Configuration

| Env Var | Default | Description |
|---|---|---|
| `SEMANTIC_PROVIDER` | _(unset)_ | `"openai"` or `"ollama"`. Unset disables the feature. |
| `OPENAI_API_KEY` | _(required when provider=openai)_ | OpenAI API key |
| `OPENAI_EMBEDDING_MODEL` | `text-embedding-3-small` | OpenAI embedding model |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Base URL for Ollama server |
| `OLLAMA_EMBEDDING_MODEL` | `nomic-embed-text` | Ollama embedding model |

## Graceful Degradation

- `SEMANTIC_PROVIDER` unset → `semantic_search` returns structured error: `"semantic search not enabled: set SEMANTIC_PROVIDER to 'openai' or 'ollama'"`. Server starts normally.
- Ollama unreachable → tool call returns descriptive error. Existing keyword search (`search_docs`) is unaffected.
- OpenAI rate limit → tool call returns descriptive error. Indexing will retry on next scan.
- Model key changes → stale vectors are ignored at query time; re-indexing runs automatically on next scan.

## Out of Scope

- No pgvector dependency; all cosine similarity computed in Go.
- No hybrid keyword+semantic blending in this phase.
- No per-user or per-team RBAC filtering (separate future item).
- No streaming responses.
