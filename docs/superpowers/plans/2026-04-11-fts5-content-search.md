# FTS5 Content Search Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the SQLite `LIKE`-based content search with SQLite FTS5 (Full-Text Search 5), delivering BM25-ranked results, Porter-stemmed tokenisation, and better snippet extraction — all with zero new dependencies.

**Architecture:** A `doc_contents_fts` FTS5 virtual table mirrors `doc_contents` via three SQL triggers (INSERT/UPDATE/DELETE). `ContentCache` gains a `useFTS5 bool` field set at construction by inspecting `db.Dialector.Name()`. `Search()` branches on that flag: SQLite uses `fts5 MATCH … ORDER BY rank`; PostgreSQL keeps the existing escaped-`LIKE` path unchanged.

**Tech Stack:** Go 1.26+, `modernc.org/sqlite` (already vendored — FTS5 confirmed available), GORM `*gorm.DB` raw SQL execution.

**Branch:** `feat/fts5-content-search` (branched from `main`)

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `memory/content.go` | Modify | `useFTS5` field; `initFTS5()` setup; FTS5 search path in `Search()` |
| `memory/content_test.go` | Modify | Add FTS5-specific tests: multi-word, BM25 ranking, stemming, snippet |

---

## Task 1: Create branch

- [ ] **Step 1: Branch from main**

```bash
cd /mnt/e/DEV/mcpdocs
git checkout main && git pull origin main
git checkout -b feat/fts5-content-search
```

Expected: `Switched to a new branch 'feat/fts5-content-search'`

---

## Task 2: FTS5 table setup + `useFTS5` field

**Files:**
- Modify: `memory/content.go`

- [ ] **Step 1: Add `useFTS5` field and `initFTS5()` to `memory/content.go`**

Find the `ContentCache` struct and `NewContentCache` function. Replace them with:

```go
// ContentCache stores and searches raw file content indexed during scans.
type ContentCache struct {
	db      *gorm.DB
	enabled bool
	maxSize int
	useFTS5 bool // true when the underlying DB is SQLite and FTS5 was initialised
}

// NewContentCache creates a ContentCache.
// enabled=false disables all writes and returns errors on Search.
// maxSize is the maximum byte size of content to store (files larger are skipped).
// FTS5 full-text search is automatically enabled when db is a SQLite connection.
func NewContentCache(db *gorm.DB, enabled bool, maxSize int) *ContentCache {
	cc := &ContentCache{db: db, enabled: enabled, maxSize: maxSize}
	if enabled && db.Dialector.Name() == "sqlite" {
		if err := cc.initFTS5(); err != nil {
			slog.Warn("[content] FTS5 setup failed, falling back to LIKE search", "error", err)
		} else {
			cc.useFTS5 = true
			slog.Info("[content] FTS5 full-text search enabled")
		}
	}
	return cc
}
```

Then add `initFTS5()` after `NewContentCache`:

```go
// initFTS5 creates the FTS5 virtual table and the three sync triggers on doc_contents.
// Safe to call multiple times — all statements use IF NOT EXISTS.
func (cc *ContentCache) initFTS5() error {
	stmts := []string{
		// FTS5 virtual table: repo_name stored but not tokenised (UNINDEXED),
		// content tokenised with Porter stemmer + ASCII case-folding.
		`CREATE VIRTUAL TABLE IF NOT EXISTS doc_contents_fts USING fts5(
			repo_name UNINDEXED,
			content,
			tokenize = 'porter ascii'
		)`,

		// Keep FTS index in sync with the main table.
		`CREATE TRIGGER IF NOT EXISTS doc_contents_fts_insert
		 AFTER INSERT ON doc_contents BEGIN
		   INSERT INTO doc_contents_fts(rowid, repo_name, content)
		   VALUES (new.id, new.repo_name, new.content);
		 END`,

		`CREATE TRIGGER IF NOT EXISTS doc_contents_fts_update
		 AFTER UPDATE ON doc_contents BEGIN
		   DELETE FROM doc_contents_fts WHERE rowid = old.id;
		   INSERT INTO doc_contents_fts(rowid, repo_name, content)
		   VALUES (new.id, new.repo_name, new.content);
		 END`,

		`CREATE TRIGGER IF NOT EXISTS doc_contents_fts_delete
		 AFTER DELETE ON doc_contents BEGIN
		   DELETE FROM doc_contents_fts WHERE rowid = old.id;
		 END`,
	}
	for _, stmt := range stmts {
		if err := cc.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("initFTS5: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./memory/...
```

Expected: no errors.

---

## Task 3: FTS5 search path in `Search()`

**Files:**
- Modify: `memory/content.go`

- [ ] **Step 1: Write the failing tests first**

Add to the bottom of `memory/content_test.go`:

```go
func TestContentCache_FTS5_MultiWordQuery(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc-a", "README.md", "sha1", "The payment service handles Stripe transactions and refunds.")
	cache.Upsert("org/svc-b", "README.md", "sha2", "Auth service manages JWT tokens.")
	cache.Upsert("org/svc-c", "README.md", "sha3", "Stripe integration for subscription billing.")

	// Multi-word query: both words must appear somewhere in the content.
	matches, err := cache.Search("stripe payment", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match for 'stripe payment'")
	}
	// svc-a has both "stripe" and "payment" — must be in the results.
	found := false
	for _, m := range matches {
		if m.RepoName == "org/svc-a" {
			found = true
		}
	}
	if !found {
		t.Errorf("org/svc-a should match 'stripe payment', got: %v", matches)
	}
}

func TestContentCache_FTS5_Stemming(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc", "README.md", "sha1", "The service handles payments for all customers.")

	// "paying" should match "payments" via Porter stemmer (both stem to "pay").
	matches, err := cache.Search("paying", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Errorf("expected stemmed match for 'paying' → 'payments', got none")
	}
}

func TestContentCache_FTS5_RankedResults(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	// svc-a mentions "authentication" once; svc-b is entirely about authentication.
	cache.Upsert("org/svc-a", "README.md", "sha1", "This service handles payments. Authentication is handled elsewhere.")
	cache.Upsert("org/svc-b", "README.md", "sha2", "Authentication service. Manages authentication tokens. Provides authentication middleware.")

	matches, err := cache.Search("authentication", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	// svc-b mentions authentication 3× — BM25 should rank it first.
	if matches[0].RepoName != "org/svc-b" {
		t.Errorf("expected org/svc-b (higher frequency) ranked first, got %s", matches[0].RepoName)
	}
}

func TestContentCache_FTS5_SnippetNotEmpty(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)
	cache.Upsert("org/svc", "README.md", "sha1", "The fraud detection service analyses transaction patterns to identify suspicious activity.")

	matches, err := cache.Search("fraud", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected match")
	}
	if matches[0].Snippet == "" {
		t.Error("snippet should not be empty")
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./memory/... -run 'TestContentCache_FTS5' -v 2>&1 | tail -20
```

Expected: `TestContentCache_FTS5_Stemming` may pass (LIKE doesn't stem, so it will fail to find "paying"). `TestContentCache_FTS5_RankedResults` will fail (LIKE has no ranking). Confirm at least one FAIL.

- [ ] **Step 3: Replace the SQLite search path in `Search()`**

In `memory/content.go`, replace the `Search()` method body with:

```go
// Search performs a full-text search across cached content.
// On SQLite: uses FTS5 with BM25 ranking and Porter stemming.
// On PostgreSQL: uses escaped LIKE (case-insensitive).
// Optionally filter by repoName (pass "" for no filter).
// Returns up to 20 results with a snippet of context around the first match.
// Returns an error if the cache is disabled.
func (cc *ContentCache) Search(query, repoName string) ([]ContentMatch, error) {
	if !cc.enabled {
		return nil, fmt.Errorf("content search is disabled: set SCAN_CONTENT=true and restart with a persistent DATABASE_URL to enable it")
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query must not be empty or whitespace-only")
	}

	if cc.useFTS5 {
		return cc.searchFTS5(query, repoName)
	}
	return cc.searchLIKE(query, repoName)
}

// searchFTS5 performs a ranked FTS5 full-text search (SQLite only).
func (cc *ContentCache) searchFTS5(query, repoName string) ([]ContentMatch, error) {
	// FTS5 snippet(): args are (table, column_index, match_start, match_end, ellipsis, tokens_per_fragment)
	// column_index 1 = content column.
	sql := `
		SELECT dc.repo_name, dc.path,
		       snippet(doc_contents_fts, 1, '', '', '...', 48) AS snippet
		FROM doc_contents_fts
		JOIN doc_contents dc ON dc.id = doc_contents_fts.rowid
		WHERE doc_contents_fts MATCH ?`
	args := []interface{}{query}

	if repoName != "" {
		sql += " AND dc.repo_name = ?"
		args = append(args, repoName)
	}
	sql += " ORDER BY rank LIMIT 20"

	type row struct {
		RepoName string
		Path     string
		Snippet  string
	}
	var rows []row
	if err := cc.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		// FTS5 MATCH syntax errors surface here — return a clear message.
		return nil, fmt.Errorf("FTS5 search failed: %w", err)
	}

	matches := make([]ContentMatch, 0, len(rows))
	for _, r := range rows {
		matches = append(matches, ContentMatch{
			RepoName: r.RepoName,
			Path:     r.Path,
			Snippet:  r.Snippet,
		})
	}
	return matches, nil
}

// searchLIKE performs a case-insensitive LIKE search (PostgreSQL fallback).
func (cc *ContentCache) searchLIKE(query, repoName string) ([]ContentMatch, error) {
	// Escape SQL LIKE special characters so the query is treated as a literal string.
	escaped := strings.ReplaceAll(query, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, "%", `\%`)
	escaped = strings.ReplaceAll(escaped, "_", `\_`)

	var rows []dbDocContent
	q := cc.db.Where("LOWER(content) LIKE LOWER(?) ESCAPE ?", "%"+escaped+"%", `\`)
	if repoName != "" {
		q = q.Where("repo_name = ?", repoName)
	}
	if err := q.Limit(20).Find(&rows).Error; err != nil {
		return nil, err
	}

	matches := make([]ContentMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, ContentMatch{
			RepoName: row.RepoName,
			Path:     row.Path,
			Snippet:  extractSnippet(row.Content, query, 300),
		})
	}
	return matches, nil
}
```

Also **remove** the old `Search()` method body that was replaced (the three methods above replace the single old `Search()` entirely).

- [ ] **Step 4: Build**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./memory/...
```

Expected: no errors.

- [ ] **Step 5: Run all FTS5 tests**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./memory/... -run 'TestContentCache_FTS5' -v 2>&1 | tail -20
```

Expected: all 4 `TestContentCache_FTS5_*` tests PASS.

- [ ] **Step 6: Run full memory test suite to confirm no regressions**

```bash
cd /mnt/e/DEV/mcpdocs && go test ./memory/... -count=1 -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL|ok)"
```

Expected: all existing tests still PASS.

- [ ] **Step 7: Commit**

```bash
git add memory/content.go memory/content_test.go
git commit -m "feat(memory): add FTS5 full-text search with BM25 ranking for SQLite"
```

---

## Task 4: Full test suite + PR

- [ ] **Step 1: Run full suite**

```bash
cd /mnt/e/DEV/mcpdocs && go build ./... && go test ./... -count=1 2>&1 | grep -E "^(ok|FAIL)"
```

Expected: all packages PASS.

- [ ] **Step 2: Create PR**

```bash
git push -u origin feat/fts5-content-search
gh pr create \
  --title "feat: FTS5 full-text content search with BM25 ranking (#2)" \
  --base main \
  --body "$(cat <<'EOF'
## Summary

- Replaces SQLite LIKE-based content search with FTS5 full-text search (zero new dependencies)
- BM25 relevance ranking: most relevant documents surface first
- Porter stemmer: \`paying\` matches \`payments\`, \`authenticate\` matches \`authentication\`
- Multi-word queries: \`stripe payment\` matches documents containing both terms
- Better snippets via SQLite \`snippet()\` function
- PostgreSQL path unchanged (LIKE fallback)

## How it works

A \`doc_contents_fts\` FTS5 virtual table mirrors \`doc_contents\` via three SQL triggers (INSERT/UPDATE/DELETE). \`ContentCache\` detects the SQLite dialect at construction and routes \`Search()\` accordingly.

## Test plan

- [x] \`TestContentCache_FTS5_MultiWordQuery\` — both terms must appear
- [x] \`TestContentCache_FTS5_Stemming\` — Porter stemmer bridges word forms
- [x] \`TestContentCache_FTS5_RankedResults\` — higher-frequency docs rank first
- [x] \`TestContentCache_FTS5_SnippetNotEmpty\` — snippet extraction works
- [x] All existing content cache tests still pass
- [x] \`go test ./...\` — full suite green

## Roadmap

Addresses roadmap item #2 (Semantic Search) — Phase 1: keyword search with stemming and relevance ranking. Vector embeddings (pgvector/sqlite-vss) remain a future phase.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review Checklist

- [x] `useFTS5` set only when `enabled=true` AND `db.Dialector.Name() == "sqlite"` — PostgreSQL unaffected
- [x] `initFTS5()` uses `IF NOT EXISTS` on all DDL — safe to call on restarts and re-connections
- [x] Triggers fire on `doc_contents` (the GORM-managed table), keeping FTS index in sync automatically
- [x] `snippet()` column index `1` matches the `content` column (index 0 = repo_name, 1 = content)
- [x] FTS5 MATCH errors (malformed query syntax) are surfaced with a clear error message
- [x] `searchLIKE()` retains LIKE wildcard escaping — PostgreSQL path is functionally unchanged
- [x] `extractSnippet()` helper kept (still used by `searchLIKE`)
- [x] TDD: tests written before implementation in Task 3
- [x] No new external dependencies
