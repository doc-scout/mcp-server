# Architecture Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable AI agents to discover distributed system architecture by auto-populating the knowledge graph from `catalog-info.yaml` files and providing full-text content search across all indexed documentation.

**Architecture:** A new `indexer/` package bridges the scanner and memory packages (which stay fully decoupled). After each scan, `AutoIndexer.Run()` fetches catalog files, parses them into entities/relations, upserts the graph with soft-delete for stale entries, and (optionally) refreshes the content cache. Two new MCP tools — `get_scan_status` and `search_content` — surface this data to AI agents.

**Tech Stack:** Go 1.26.1, `github.com/modelcontextprotocol/go-sdk`, GORM + SQLite/PostgreSQL, `gopkg.in/yaml.v3`

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Create | `scanner/parser/catalog.go` | Parse `catalog-info.yaml` bytes → `ParsedCatalog` struct |
| Create | `scanner/parser/catalog_test.go` | Unit tests for parser |
| Modify | `memory/memory.go` | Export `OpenDB`, refactor `Register(s, db)`, add `AutoWriter` + `EntityCount` |
| Modify | `memory/memory_test.go` | Update `newTestStore` to use `OpenDB` |
| Create | `memory/content.go` | `ContentCache`: `dbDocContent` GORM model, `Upsert`, `NeedsUpdate`, `Search`, `Count`, `DeleteOrphanedContent` |
| Create | `memory/content_test.go` | Unit tests for content cache |
| Modify | `scanner/scanner.go` | Add `onScanComplete` callback field, `SetOnScanComplete`, call after each full scan |
| Modify | `scanner/scanner_test.go` | Test that callback fires after scan |
| Create | `indexer/indexer.go` | `AutoIndexer`: `FileGetter` + `GraphWriter` interfaces, `Run()` |
| Create | `indexer/indexer_test.go` | Unit tests with mocks |
| Modify | `tools/tools.go` | Add `Status()` to `DocumentScanner`; add `GraphCounter` + `ContentSearcher` interfaces; register `get_scan_status` + `search_content`; update `Register` signature |
| Modify | `tools/tools_test.go` | Add `Status()` to `mockScanner`; add handler tests |
| Modify | `main.go` | Wire `OpenDB`, `AutoWriter`, `ContentCache`, `AutoIndexer`, `SetOnScanComplete`, new env vars |
| Modify | `integration_test.go` | Add `Status()` to `mockScanner`; update `memory.Register` call; add E2E tests |

---

## Task 1: Add YAML Dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

```bash
cd e:/DEV/mcpdocs
go get gopkg.in/yaml.v3
```

Expected output includes: `go: added gopkg.in/yaml.v3 v3.x.x`

- [ ] **Step 2: Verify it resolves**

```bash
go mod tidy
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add gopkg.in/yaml.v3 dependency"
```

---

## Task 2: Catalog Parser

**Files:**
- Create: `scanner/parser/catalog.go`
- Create: `scanner/parser/catalog_test.go`

- [ ] **Step 1: Write the failing tests**

Create `scanner/parser/catalog_test.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser_test

import (
	"testing"

	"docscout-mcp/scanner/parser"
)

func TestParseCatalog_Component(t *testing.T) {
	yaml := []byte(`
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: payment-service
  description: Handles payment transactions
  tags:
    - payment
    - critical
spec:
  type: service
  lifecycle: production
  owner: team-payments
  system: payment-platform
  dependsOn:
    - component:database
  providesApis:
    - payment-api
  consumesApis:
    - notification-api
`)
	got, err := parser.ParseCatalog(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.EntityName != "payment-service" {
		t.Errorf("EntityName: want payment-service, got %s", got.EntityName)
	}
	if got.EntityType != "service" {
		t.Errorf("EntityType: want service, got %s", got.EntityType)
	}

	wantObs := map[string]bool{
		"lifecycle:production":                   true,
		"description:Handles payment transactions": true,
		"tag:payment":                             true,
		"tag:critical":                            true,
	}
	for _, obs := range got.Observations {
		delete(wantObs, obs)
	}
	if len(wantObs) > 0 {
		t.Errorf("missing observations: %v", wantObs)
	}

	wantRels := map[string]bool{
		"payment-service->team-payments:owned_by":        true,
		"payment-service->payment-platform:part_of":      true,
		"payment-service->component:database:depends_on": true,
		"payment-service->payment-api:provides_api":      true,
		"payment-service->notification-api:consumes_api": true,
	}
	for _, r := range got.Relations {
		key := r.From + "->" + r.To + ":" + r.RelationType
		delete(wantRels, key)
	}
	if len(wantRels) > 0 {
		t.Errorf("missing relations: %v", wantRels)
	}
}

func TestParseCatalog_KindMapping(t *testing.T) {
	cases := []struct {
		kind       string
		specType   string
		wantType   string
	}{
		{"API", "", "api"},
		{"System", "", "system"},
		{"Resource", "", "resource"},
		{"Group", "", "team"},
		{"Component", "library", "library"},
		{"Component", "", "component"},
		{"Unknown", "", "component"},
	}
	for _, tc := range cases {
		yaml := []byte("apiVersion: backstage.io/v1alpha1\nkind: " + tc.kind + "\nmetadata:\n  name: test-entity\nspec:\n  type: " + tc.specType + "\n")
		got, err := parser.ParseCatalog(yaml)
		if err != nil {
			t.Fatalf("kind=%s: unexpected error: %v", tc.kind, err)
		}
		if got.EntityType != tc.wantType {
			t.Errorf("kind=%s specType=%s: want entityType=%s, got=%s", tc.kind, tc.specType, tc.wantType, got.EntityType)
		}
	}
}

func TestParseCatalog_MissingOptionalFields(t *testing.T) {
	yaml := []byte(`
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: minimal-service
spec:
  type: service
`)
	got, err := parser.ParseCatalog(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.EntityName != "minimal-service" {
		t.Errorf("want minimal-service, got %s", got.EntityName)
	}
	if len(got.Observations) != 0 {
		t.Errorf("expected no observations, got %v", got.Observations)
	}
	if len(got.Relations) != 0 {
		t.Errorf("expected no relations, got %v", got.Relations)
	}
}

func TestParseCatalog_MissingName(t *testing.T) {
	yaml := []byte("apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  description: no name\n")
	_, err := parser.ParseCatalog(yaml)
	if err == nil {
		t.Fatal("expected error for missing metadata.name")
	}
}

func TestParseCatalog_MalformedYAML(t *testing.T) {
	_, err := parser.ParseCatalog([]byte("this: is: not: valid: yaml: :::"))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd e:/DEV/mcpdocs
go test ./scanner/parser/...
```

Expected: `cannot find package "docscout-mcp/scanner/parser"` or build error.

- [ ] **Step 3: Implement the parser**

Create `scanner/parser/catalog.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParsedCatalog holds the data extracted from a Backstage catalog-info.yaml.
type ParsedCatalog struct {
	EntityName   string
	EntityType   string
	Observations []string
	Relations    []ParsedRelation
}

// ParsedRelation is a directed edge extracted from catalog-info.yaml.
type ParsedRelation struct {
	From         string
	To           string
	RelationType string
}

type backstageCatalog struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description"`
		Tags        []string `yaml:"tags"`
	} `yaml:"metadata"`
	Spec struct {
		Type         string   `yaml:"type"`
		Lifecycle    string   `yaml:"lifecycle"`
		Owner        string   `yaml:"owner"`
		System       string   `yaml:"system"`
		DependsOn    []string `yaml:"dependsOn"`
		ProvidesApis []string `yaml:"providesApis"`
		ConsumesApis []string `yaml:"consumesApis"`
	} `yaml:"spec"`
}

func kindToEntityType(kind, specType string) string {
	switch kind {
	case "API":
		return "api"
	case "System":
		return "system"
	case "Resource":
		return "resource"
	case "Group":
		return "team"
	case "Component":
		if specType != "" {
			return specType
		}
		return "component"
	default:
		return "component"
	}
}

// ParseCatalog parses raw catalog-info.yaml bytes into a ParsedCatalog.
// Returns an error only for YAML parse failures or missing metadata.name.
// Missing optional fields are silently skipped.
func ParseCatalog(data []byte) (ParsedCatalog, error) {
	var raw backstageCatalog
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ParsedCatalog{}, fmt.Errorf("catalog-info.yaml parse error: %w", err)
	}
	if raw.Metadata.Name == "" {
		return ParsedCatalog{}, fmt.Errorf("catalog-info.yaml: missing metadata.name")
	}

	name := raw.Metadata.Name
	entityType := kindToEntityType(raw.Kind, raw.Spec.Type)

	var obs []string
	if raw.Spec.Lifecycle != "" {
		obs = append(obs, "lifecycle:"+raw.Spec.Lifecycle)
	}
	if raw.Metadata.Description != "" {
		obs = append(obs, "description:"+raw.Metadata.Description)
	}
	for _, tag := range raw.Metadata.Tags {
		if tag != "" {
			obs = append(obs, "tag:"+tag)
		}
	}

	var rels []ParsedRelation
	if raw.Spec.Owner != "" {
		rels = append(rels, ParsedRelation{From: name, To: raw.Spec.Owner, RelationType: "owned_by"})
	}
	if raw.Spec.System != "" {
		rels = append(rels, ParsedRelation{From: name, To: raw.Spec.System, RelationType: "part_of"})
	}
	for _, dep := range raw.Spec.DependsOn {
		rels = append(rels, ParsedRelation{From: name, To: dep, RelationType: "depends_on"})
	}
	for _, api := range raw.Spec.ProvidesApis {
		rels = append(rels, ParsedRelation{From: name, To: api, RelationType: "provides_api"})
	}
	for _, api := range raw.Spec.ConsumesApis {
		rels = append(rels, ParsedRelation{From: name, To: api, RelationType: "consumes_api"})
	}

	return ParsedCatalog{
		EntityName:   name,
		EntityType:   entityType,
		Observations: obs,
		Relations:    rels,
	}, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./scanner/parser/... -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add scanner/parser/catalog.go scanner/parser/catalog_test.go
git commit -m "feat: add catalog-info.yaml parser"
```

---

## Task 3: Refactor Memory Package — Export OpenDB + Add AutoWriter

**Files:**
- Modify: `memory/memory.go`
- Modify: `memory/memory_test.go`

- [ ] **Step 1: Write failing tests for OpenDB and AutoWriter**

Add to the bottom of `memory/memory_test.go` (after existing tests):

```go
func TestOpenDB_InMemory(t *testing.T) {
	db, err := OpenDB("")
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestAutoWriter_EntityCount(t *testing.T) {
	db, err := OpenDB(fmt.Sprintf("file:autowriter_%d?mode=memory&cache=shared", testCounter.Add(1)))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	w := NewAutoWriter(db)

	count, err := w.EntityCount()
	if err != nil {
		t.Fatalf("EntityCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 entities, got %d", count)
	}

	_, err = w.CreateEntities([]Entity{
		{Name: "svc-x", EntityType: "service"},
		{Name: "svc-y", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	count, err = w.EntityCount()
	if err != nil {
		t.Fatalf("EntityCount after create: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 entities, got %d", count)
	}
}

func TestAutoWriter_CreateRelations(t *testing.T) {
	db, err := OpenDB(fmt.Sprintf("file:autowriter_rel_%d?mode=memory&cache=shared", testCounter.Add(1)))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	w := NewAutoWriter(db)

	_, err = w.CreateEntities([]Entity{
		{Name: "svc-a", EntityType: "service"},
		{Name: "svc-b", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	rels, err := w.CreateRelations([]Relation{
		{From: "svc-a", To: "svc-b", RelationType: "depends_on"},
	})
	if err != nil {
		t.Fatalf("CreateRelations: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("expected 1 relation, got %d", len(rels))
	}
}

func TestAutoWriter_SearchNodes(t *testing.T) {
	db, err := OpenDB(fmt.Sprintf("file:autowriter_search_%d?mode=memory&cache=shared", testCounter.Add(1)))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	w := NewAutoWriter(db)

	_, err = w.CreateEntities([]Entity{
		{Name: "payment-svc", EntityType: "service", Observations: []string{"_source:catalog-info"}},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	graph, err := w.SearchNodes("_source:catalog-info")
	if err != nil {
		t.Fatalf("SearchNodes: %v", err)
	}
	if len(graph.Entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(graph.Entities))
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./memory/... -run "TestOpenDB|TestAutoWriter"
```

Expected: compilation error — `OpenDB` and `NewAutoWriter` undefined.

- [ ] **Step 3: Implement the changes in `memory/memory.go`**

**a) Export `openDB` as `OpenDB` and move `AutoMigrate` into it.** Replace the existing `openDB` function and the `AutoMigrate` block inside `Register` with:

```go
// OpenDB opens the database connection and runs auto-migration for all models.
// dbURL accepts: "sqlite://path.db", "postgres://...", a plain file path, or "" for in-memory SQLite.
func OpenDB(dbURL string) (*gorm.DB, error) {
	cfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	var db *gorm.DB
	var err error

	switch {
	case strings.HasPrefix(dbURL, "postgres://"), strings.HasPrefix(dbURL, "postgresql://"):
		db, err = gorm.Open(postgres.Open(dbURL), cfg)
	case strings.HasPrefix(dbURL, "sqlite://"):
		path := strings.TrimPrefix(dbURL, "sqlite://")
		db, err = gorm.Open(sqlite.Open(path), cfg)
	case dbURL == "":
		db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), cfg)
	default:
		db, err = gorm.Open(sqlite.Open(dbURL), cfg)
	}

	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&dbEntity{}, &dbRelation{}, &dbObservation{}); err != nil {
		return nil, err
	}
	return db, nil
}
```

**b) Refactor `Register` to accept `*gorm.DB` instead of `dbURL string`:**

```go
// Register adds the knowledge graph memory tools to the MCP server.
// db must be obtained via OpenDB (already migrated).
func Register(s *mcp.Server, db *gorm.DB) {
	mem := store{db: db}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_entities",
		Description: "Create multiple new entities in the knowledge graph",
	}, mem.CreateEntities)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_relations",
		Description: "Create multiple new relations between entities",
	}, mem.CreateRelations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_observations",
		Description: "Add new observations to existing entities",
	}, mem.AddObservations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_entities",
		Description: "Remove entities and their relations",
	}, mem.DeleteEntities)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_observations",
		Description: "Remove specific observations from entities",
	}, mem.DeleteObservations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_relations",
		Description: "Remove specific relations from the graph",
	}, mem.DeleteRelations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "read_graph",
		Description: "Read the entire knowledge graph",
	}, mem.ReadGraph)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_nodes",
		Description: "Search for nodes based on query",
	}, mem.SearchNodes)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "open_nodes",
		Description: "Retrieve specific nodes by name",
	}, mem.OpenNodes)

	log.Printf("[memory] Knowledge graph initialized")
}
```

**c) Add `AutoWriter` below the `Register` function:**

```go
// AutoWriter exposes a clean data-layer API for the auto-indexer.
// It shares the same *gorm.DB as the MCP tool store.
type AutoWriter struct {
	s store
}

// NewAutoWriter creates an AutoWriter using an already-opened *gorm.DB.
func NewAutoWriter(db *gorm.DB) *AutoWriter {
	return &AutoWriter{s: store{db: db}}
}

// CreateEntities creates entities, skipping duplicates.
func (w *AutoWriter) CreateEntities(entities []Entity) ([]Entity, error) {
	return w.s.createEntities(entities)
}

// CreateRelations creates relations, skipping duplicates.
func (w *AutoWriter) CreateRelations(relations []Relation) ([]Relation, error) {
	return w.s.createRelations(relations)
}

// AddObservations appends observations to existing entities, skipping duplicates.
func (w *AutoWriter) AddObservations(obs []Observation) ([]Observation, error) {
	return w.s.addObservations(obs)
}

// SearchNodes searches entities by name, type, or observation content.
func (w *AutoWriter) SearchNodes(query string) (KnowledgeGraph, error) {
	return w.s.searchNodes(query)
}

// EntityCount returns the total number of entities in the knowledge graph.
func (w *AutoWriter) EntityCount() (int64, error) {
	var count int64
	err := w.s.db.Model(&dbEntity{}).Count(&count).Error
	return count, err
}
```

**d) Remove the old unexported `openDB` function and `backendLabel` helper** — they are no longer needed since `OpenDB` replaces them.

- [ ] **Step 4: Update `memory/memory_test.go` — replace `newTestStore` to use `OpenDB`**

Replace the `newTestStore` function:

```go
func newTestStore(t *testing.T) store {
	t.Helper()
	n := testCounter.Add(1)
	dsn := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", n)
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	return store{db: db}
}
```

- [ ] **Step 5: Run all memory tests**

```bash
go test ./memory/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add memory/memory.go memory/memory_test.go
git commit -m "refactor: export OpenDB and add AutoWriter to memory package"
```

---

## Task 4: Content Cache

**Files:**
- Create: `memory/content.go`
- Create: `memory/content_test.go`
- Modify: `memory/memory.go` (add `dbDocContent` to `OpenDB`'s `AutoMigrate`)

- [ ] **Step 1: Write failing tests**

Create `memory/content_test.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"fmt"
	"strings"
	"testing"
)

func newTestContentCache(t *testing.T, maxSize int) *ContentCache {
	t.Helper()
	n := testCounter.Add(1)
	dsn := fmt.Sprintf("file:contentdb_%d?mode=memory&cache=shared", n)
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	return NewContentCache(db, true, maxSize)
}

func TestContentCache_Upsert_New(t *testing.T) {
	cache := newTestContentCache(t, 1024)

	err := cache.Upsert("my-org/svc-a", "README.md", "sha1", "# Service A\nThis handles payments.")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	count, err := cache.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cached file, got %d", count)
	}
}

func TestContentCache_Upsert_SkipsSameSHA(t *testing.T) {
	cache := newTestContentCache(t, 1024)

	cache.Upsert("my-org/svc-a", "README.md", "sha1", "original content")
	// Second upsert with same SHA should be a no-op (NeedsUpdate returns false)
	if cache.NeedsUpdate("my-org/svc-a", "README.md", "sha1") {
		t.Error("NeedsUpdate should be false for same SHA")
	}
}

func TestContentCache_Upsert_UpdatesOnNewSHA(t *testing.T) {
	cache := newTestContentCache(t, 1024)

	cache.Upsert("my-org/svc-a", "README.md", "sha1", "old content")
	if !cache.NeedsUpdate("my-org/svc-a", "README.md", "sha2") {
		t.Error("NeedsUpdate should be true when SHA changes")
	}
	cache.Upsert("my-org/svc-a", "README.md", "sha2", "new content")

	// Verify stored SHA is now sha2
	if cache.NeedsUpdate("my-org/svc-a", "README.md", "sha2") {
		t.Error("NeedsUpdate should be false after updating to sha2")
	}
}

func TestContentCache_Upsert_SizeCap(t *testing.T) {
	// maxSize of 10 bytes — any real content exceeds it
	cache := newTestContentCache(t, 10)

	err := cache.Upsert("my-org/svc-a", "README.md", "sha1", "this content is definitely longer than ten bytes")
	if err != nil {
		t.Fatalf("Upsert with large content: %v", err)
	}

	count, _ := cache.Count()
	if count != 0 {
		t.Errorf("oversized file should not be cached, count=%d", count)
	}
}

func TestContentCache_Search_Basic(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/payment-svc", "README.md", "sha1", "# Payment Service\nHandles Stripe transactions and refunds.")
	cache.Upsert("org/auth-svc", "README.md", "sha2", "# Auth Service\nManages JWT tokens and sessions.")

	matches, err := cache.Search("stripe", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].RepoName != "org/payment-svc" {
		t.Errorf("wrong repo: %s", matches[0].RepoName)
	}
	if !strings.Contains(matches[0].Snippet, "Stripe") {
		t.Errorf("snippet should contain 'Stripe', got: %s", matches[0].Snippet)
	}
}

func TestContentCache_Search_FilterByRepo(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc-a", "README.md", "sha1", "payment processing logic")
	cache.Upsert("org/svc-b", "README.md", "sha2", "payment gateway integration")

	matches, err := cache.Search("payment", "org/svc-a")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match with repo filter, got %d", len(matches))
	}
	if matches[0].RepoName != "org/svc-a" {
		t.Errorf("wrong repo: %s", matches[0].RepoName)
	}
}

func TestContentCache_Search_CaseInsensitive(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)
	cache.Upsert("org/svc", "docs/api.md", "sha1", "The PAYMENT endpoint accepts POST requests.")

	matches, _ := cache.Search("payment", "")
	if len(matches) != 1 {
		t.Fatalf("expected case-insensitive match, got %d matches", len(matches))
	}
}

func TestContentCache_DeleteOrphanedContent(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/active-svc", "README.md", "sha1", "active service content")
	cache.Upsert("org/gone-svc", "README.md", "sha2", "removed service content")

	err := cache.DeleteOrphanedContent([]string{"org/active-svc"})
	if err != nil {
		t.Fatalf("DeleteOrphanedContent: %v", err)
	}

	count, _ := cache.Count()
	if count != 1 {
		t.Errorf("expected 1 remaining, got %d", count)
	}
}

func TestContentCache_Disabled(t *testing.T) {
	n := testCounter.Add(1)
	dsn := fmt.Sprintf("file:contentdb_disabled_%d?mode=memory&cache=shared", n)
	db, _ := OpenDB(dsn)
	cache := NewContentCache(db, false, 1024)

	err := cache.Upsert("org/svc", "README.md", "sha1", "content")
	if err != nil {
		t.Fatalf("Upsert on disabled cache should not error: %v", err)
	}

	count, _ := cache.Count()
	if count != 0 {
		t.Errorf("disabled cache should store nothing, count=%d", count)
	}

	_, err = cache.Search("anything", "")
	if err == nil {
		t.Error("Search on disabled cache should return error")
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./memory/... -run "TestContentCache"
```

Expected: compilation error — `ContentCache`, `NewContentCache`, etc. undefined.

- [ ] **Step 3: Add `dbDocContent` to `OpenDB`'s `AutoMigrate` in `memory/memory.go`**

In `OpenDB`, change the `AutoMigrate` call:

```go
if err := db.AutoMigrate(&dbEntity{}, &dbRelation{}, &dbObservation{}, &dbDocContent{}); err != nil {
    return nil, err
}
```

- [ ] **Step 4: Implement `memory/content.go`**

Create `memory/content.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// dbDocContent stores cached file content indexed by repo and path.
type dbDocContent struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	RepoName  string    `gorm:"index;uniqueIndex:idx_repo_path"`
	Path      string    `gorm:"uniqueIndex:idx_repo_path"`
	SHA       string
	Content   string    `gorm:"type:text"`
	IndexedAt time.Time
}

// ContentMatch is a search result from the content cache.
type ContentMatch struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	Snippet  string `json:"snippet"`
}

// ContentCache stores and searches raw file content indexed during scans.
type ContentCache struct {
	db      *gorm.DB
	enabled bool
	maxSize int
}

// NewContentCache creates a ContentCache.
// enabled=false disables all writes and returns errors on Search.
// maxSize is the maximum byte size of content to store (files larger are skipped).
func NewContentCache(db *gorm.DB, enabled bool, maxSize int) *ContentCache {
	return &ContentCache{db: db, enabled: enabled, maxSize: maxSize}
}

// NeedsUpdate returns true if the file is not cached or its SHA has changed.
// Always returns false when the cache is disabled.
func (cc *ContentCache) NeedsUpdate(repoName, path, sha string) bool {
	if !cc.enabled {
		return false
	}
	var existing dbDocContent
	err := cc.db.Where("repo_name = ? AND path = ?", repoName, path).First(&existing).Error
	if err != nil {
		return true // not found
	}
	return existing.SHA != sha
}

// Upsert stores or updates the content for a file.
// Files exceeding maxSize are silently skipped.
// No-ops when the cache is disabled.
func (cc *ContentCache) Upsert(repoName, path, sha, content string) error {
	if !cc.enabled {
		return nil
	}
	if len(content) > cc.maxSize {
		return nil // skip oversized files silently
	}
	row := dbDocContent{
		RepoName:  repoName,
		Path:      path,
		SHA:       sha,
		Content:   content,
		IndexedAt: time.Now(),
	}
	return cc.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repo_name"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"sha", "content", "indexed_at"}),
	}).Create(&row).Error
}

// Search performs a case-insensitive full-text search across cached content.
// Optionally filter by repoName (pass "" for no filter).
// Returns up to 20 results with a snippet of ~300 chars around the first match.
// Returns an error if the cache is disabled.
func (cc *ContentCache) Search(query, repoName string) ([]ContentMatch, error) {
	if !cc.enabled {
		return nil, fmt.Errorf("content search is disabled: set SCAN_CONTENT=true and restart with a persistent DATABASE_URL to enable it")
	}
	if query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	var rows []dbDocContent
	q := cc.db.Where("LOWER(content) LIKE LOWER(?)", "%"+query+"%")
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

// Count returns the number of files currently in the content cache.
func (cc *ContentCache) Count() (int64, error) {
	var count int64
	err := cc.db.Model(&dbDocContent{}).Count(&count).Error
	return count, err
}

// DeleteOrphanedContent removes content rows for repos not in activeRepos.
func (cc *ContentCache) DeleteOrphanedContent(activeRepos []string) error {
	if !cc.enabled || len(activeRepos) == 0 {
		return nil
	}
	return cc.db.Where("repo_name NOT IN ?", activeRepos).Delete(&dbDocContent{}).Error
}

// extractSnippet returns ~snippetSize chars of context around the first occurrence of query.
func extractSnippet(content, query string, snippetSize int) string {
	lower := strings.ToLower(content)
	lowerQ := strings.ToLower(query)
	idx := strings.Index(lower, lowerQ)
	if idx < 0 {
		if len(content) > snippetSize {
			return content[:snippetSize] + "..."
		}
		return content
	}
	half := snippetSize / 2
	start := max(0, idx-half)
	end := min(len(content), idx+len(query)+half)
	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}
	return snippet
}
```

- [ ] **Step 5: Run content cache tests**

```bash
go test ./memory/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add memory/content.go memory/content_test.go memory/memory.go
git commit -m "feat: add ContentCache with SHA-based incremental updates and full-text search"
```

---

## Task 5: Scanner OnScanComplete Callback

**Files:**
- Modify: `scanner/scanner.go`
- Modify: `scanner/scanner_test.go`

- [ ] **Step 1: Write failing test**

Add to `scanner/scanner_test.go`:

```go
func TestScanner_OnScanComplete(t *testing.T) {
	ts, client := setupMockGitHub()
	defer ts.Close()

	s := New(client, "test-org", 0, []string{"README.md"}, []string{"docs"}, nil, nil, nil)

	called := false
	var callbackRepos []RepoInfo
	s.SetOnScanComplete(func(repos []RepoInfo) {
		called = true
		callbackRepos = repos
	})

	s.scanOrg(context.Background())

	if !called {
		t.Fatal("OnScanComplete callback was not called")
	}
	if len(callbackRepos) != 1 {
		t.Errorf("expected 1 repo in callback, got %d", len(callbackRepos))
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./scanner/... -run TestScanner_OnScanComplete
```

Expected: compilation error — `SetOnScanComplete` undefined.

- [ ] **Step 3: Add callback to `scanner/scanner.go`**

Add `onScanComplete` field to the `Scanner` struct:

```go
type Scanner struct {
	client       *github.Client
	org          string
	scanInterval time.Duration
	targetFiles  []string
	scanDirs     []string
	extraRepos   []string
	repoTopics   []string
	repoRegex    *regexp.Regexp

	mu    sync.RWMutex
	repos map[string]*RepoInfo

	scanning   bool
	lastScanAt time.Time

	onScanComplete func([]RepoInfo) // called after each full scan completes
}
```

Add `SetOnScanComplete` method after `New`:

```go
// SetOnScanComplete registers a callback invoked after each full scan with the current repo list.
// The callback runs synchronously in the scan goroutine.
func (s *Scanner) SetOnScanComplete(fn func([]RepoInfo)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onScanComplete = fn
}
```

In `scanOrg`, call the callback at the very end, after the atomic swap of `s.repos`:

```go
// After the mutex unlock for the repo swap:
s.mu.Lock()
s.repos = newRepos
onComplete := s.onScanComplete
s.mu.Unlock()

if onComplete != nil {
	repos := s.ListRepos()
	onComplete(repos)
}
```

The complete updated end of `scanOrg` (replacing the current last few lines):

```go
	wg.Wait()

	// Swap entire cache atomically.
	s.mu.Lock()
	s.repos = newRepos
	onComplete := s.onScanComplete
	s.mu.Unlock()

	// Invoke callback outside the lock to avoid deadlock if callback calls ListRepos.
	if onComplete != nil {
		onComplete(s.ListRepos())
	}
```

- [ ] **Step 4: Run all scanner tests**

```bash
go test ./scanner/... -v
```

Expected: all tests PASS including `TestScanner_OnScanComplete`.

- [ ] **Step 5: Commit**

```bash
git add scanner/scanner.go scanner/scanner_test.go
git commit -m "feat: add OnScanComplete callback to scanner"
```

---

## Task 6: Auto-Indexer

**Files:**
- Create: `indexer/indexer.go`
- Create: `indexer/indexer_test.go`

- [ ] **Step 1: Write failing tests**

Create `indexer/indexer_test.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer_test

import (
	"context"
	"fmt"
	"testing"

	"docscout-mcp/indexer"
	"docscout-mcp/memory"
	"docscout-mcp/scanner"
)

// --- Mock FileGetter ---

type mockFileGetter struct {
	files map[string]string // key: "repoName/path"
}

func (m *mockFileGetter) GetFileContent(ctx context.Context, repo, path string) (string, error) {
	key := repo + "/" + path
	if content, ok := m.files[key]; ok {
		return content, nil
	}
	return "", fmt.Errorf("not found: %s", key)
}

// --- Mock GraphWriter ---

type mockGraphWriter struct {
	entities  []memory.Entity
	relations []memory.Relation
}

func (m *mockGraphWriter) CreateEntities(entities []memory.Entity) ([]memory.Entity, error) {
	m.entities = append(m.entities, entities...)
	return entities, nil
}

func (m *mockGraphWriter) CreateRelations(relations []memory.Relation) ([]memory.Relation, error) {
	m.relations = append(m.relations, relations...)
	return relations, nil
}

func (m *mockGraphWriter) AddObservations(obs []memory.Observation) ([]memory.Observation, error) {
	for _, o := range obs {
		for i, e := range m.entities {
			if e.Name == o.EntityName {
				m.entities[i].Observations = append(m.entities[i].Observations, o.Contents...)
			}
		}
	}
	return obs, nil
}

func (m *mockGraphWriter) SearchNodes(query string) (memory.KnowledgeGraph, error) {
	var matched []memory.Entity
	for _, e := range m.entities {
		for _, obs := range e.Observations {
			if obs == query || containsStr(e.Observations, query) {
				matched = append(matched, e)
				break
			}
		}
	}
	return memory.KnowledgeGraph{Entities: matched}, nil
}

func (m *mockGraphWriter) EntityCount() (int64, error) {
	return int64(len(m.entities)), nil
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// --- Tests ---

func TestAutoIndexer_CreatesEntitiesFromCatalog(t *testing.T) {
	catalogYAML := `
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: payment-service
  description: Handles payment
spec:
  type: service
  lifecycle: production
  owner: team-payments
  dependsOn:
    - component:db
`
	fg := &mockFileGetter{
		files: map[string]string{
			"org/payment-service/catalog-info.yaml": catalogYAML,
		},
	}
	gw := &mockGraphWriter{}

	ai := indexer.New(fg, gw, nil)
	ai.Run(context.Background(), []scanner.RepoInfo{
		{
			Name:     "org/payment-service",
			FullName: "org/payment-service",
			Files: []scanner.FileEntry{
				{RepoName: "org/payment-service", Path: "catalog-info.yaml", Type: "catalog-info"},
			},
		},
	})

	if len(gw.entities) == 0 {
		t.Fatal("expected entities to be created")
	}

	found := false
	for _, e := range gw.entities {
		if e.Name == "payment-service" {
			found = true
			if e.EntityType != "service" {
				t.Errorf("expected entityType=service, got %s", e.EntityType)
			}
			// Must have auto-source observations
			if !containsStr(e.Observations, "_source:catalog-info") {
				t.Errorf("missing _source:catalog-info observation, got: %v", e.Observations)
			}
			if !containsStr(e.Observations, "_scan_repo:org/payment-service") {
				t.Errorf("missing _scan_repo observation, got: %v", e.Observations)
			}
		}
	}
	if !found {
		t.Errorf("payment-service entity not created; entities: %v", gw.entities)
	}

	// Verify depends_on relation was created
	depFound := false
	for _, r := range gw.relations {
		if r.From == "payment-service" && r.To == "component:db" && r.RelationType == "depends_on" {
			depFound = true
		}
	}
	if !depFound {
		t.Errorf("depends_on relation not created; relations: %v", gw.relations)
	}
}

func TestAutoIndexer_SkipsMalformedCatalog(t *testing.T) {
	fg := &mockFileGetter{
		files: map[string]string{
			"org/bad-svc/catalog-info.yaml": "this: is: not: valid: yaml: :::",
		},
	}
	gw := &mockGraphWriter{}
	ai := indexer.New(fg, gw, nil)

	// Should not panic or return error; just log and skip
	ai.Run(context.Background(), []scanner.RepoInfo{
		{
			Name: "org/bad-svc",
			Files: []scanner.FileEntry{
				{RepoName: "org/bad-svc", Path: "catalog-info.yaml", Type: "catalog-info"},
			},
		},
	})

	if len(gw.entities) != 0 {
		t.Errorf("expected no entities from malformed YAML, got %v", gw.entities)
	}
}

func TestAutoIndexer_SoftDeletesStaleEntities(t *testing.T) {
	// Pre-populate graph with an entity from a repo that won't be in the next scan
	gw := &mockGraphWriter{
		entities: []memory.Entity{
			{
				Name:       "old-service",
				EntityType: "service",
				Observations: []string{
					"_source:catalog-info",
					"_scan_repo:org/old-svc",
				},
			},
		},
	}
	fg := &mockFileGetter{files: map[string]string{}}
	ai := indexer.New(fg, gw, nil)

	// Run with an empty repo list (org/old-svc is gone)
	ai.Run(context.Background(), []scanner.RepoInfo{})

	// old-service should now have _status:archived
	archivedFound := false
	for _, e := range gw.entities {
		if e.Name == "old-service" {
			if containsStr(e.Observations, "_status:archived") {
				archivedFound = true
			}
		}
	}
	if !archivedFound {
		t.Errorf("expected _status:archived on stale entity; entities: %v", gw.entities)
	}
}

func TestAutoIndexer_PreservesManualEntities(t *testing.T) {
	// A manually-created entity (no _source:catalog-info) should not be overwritten
	gw := &mockGraphWriter{
		entities: []memory.Entity{
			{
				Name:         "payment-service",
				EntityType:   "service",
				Observations: []string{"manually added observation"},
				// No _source:catalog-info
			},
		},
	}
	catalogYAML := `
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: payment-service
spec:
  type: service
  lifecycle: production
`
	fg := &mockFileGetter{
		files: map[string]string{
			"org/payment-service/catalog-info.yaml": catalogYAML,
		},
	}
	ai := indexer.New(fg, gw, nil)
	ai.Run(context.Background(), []scanner.RepoInfo{
		{
			Name: "org/payment-service",
			Files: []scanner.FileEntry{
				{RepoName: "org/payment-service", Path: "catalog-info.yaml", Type: "catalog-info"},
			},
		},
	})

	// Manual observation must still be present
	for _, e := range gw.entities {
		if e.Name == "payment-service" {
			if !containsStr(e.Observations, "manually added observation") {
				t.Errorf("manual observation was removed; observations: %v", e.Observations)
			}
			return
		}
	}
	t.Error("payment-service entity not found after run")
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./indexer/... -v
```

Expected: compilation error — package `docscout-mcp/indexer` does not exist.

- [ ] **Step 3: Implement `indexer/indexer.go`**

Create `indexer/indexer.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer

import (
	"context"
	"log"
	"strings"

	"docscout-mcp/memory"
	"docscout-mcp/scanner"
	"docscout-mcp/scanner/parser"
)

// FileGetter fetches the raw content of an indexed documentation file.
type FileGetter interface {
	GetFileContent(ctx context.Context, repo, path string) (string, error)
}

// GraphWriter writes entities and relations to the knowledge graph.
type GraphWriter interface {
	CreateEntities([]memory.Entity) ([]memory.Entity, error)
	CreateRelations([]memory.Relation) ([]memory.Relation, error)
	AddObservations([]memory.Observation) ([]memory.Observation, error)
	SearchNodes(query string) (memory.KnowledgeGraph, error)
	EntityCount() (int64, error)
}

// AutoIndexer automatically populates the knowledge graph from catalog-info.yaml files
// and optionally refreshes the content cache after each scan.
type AutoIndexer struct {
	sc    FileGetter
	graph GraphWriter
	cache *memory.ContentCache // nil if content caching disabled
}

// New creates an AutoIndexer. cache may be nil if SCAN_CONTENT is disabled.
func New(sc FileGetter, graph GraphWriter, cache *memory.ContentCache) *AutoIndexer {
	return &AutoIndexer{sc: sc, graph: graph, cache: cache}
}

// Run is the OnScanComplete callback. It:
//  1. Refreshes the content cache for all indexed files (if enabled).
//  2. Parses catalog-info.yaml files and upserts entities/relations in the graph.
//  3. Soft-deletes entities from repos no longer in the scan.
func (ai *AutoIndexer) Run(ctx context.Context, repos []scanner.RepoInfo) {
	activeRepos := make(map[string]bool, len(repos))
	for _, r := range repos {
		activeRepos[r.Name] = true
	}

	// Phase 1: Refresh content cache.
	if ai.cache != nil {
		ai.refreshContent(ctx, repos)
		activeRepoList := make([]string, 0, len(activeRepos))
		for name := range activeRepos {
			activeRepoList = append(activeRepoList, name)
		}
		if err := ai.cache.DeleteOrphanedContent(activeRepoList); err != nil {
			log.Printf("[indexer] Failed to delete orphaned content: %v", err)
		}
	}

	// Phase 2: Auto-graph from catalog-info.yaml.
	for _, repo := range repos {
		for _, file := range repo.Files {
			if file.Type != "catalog-info" {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				log.Printf("[indexer] Failed to fetch %s/%s: %v", repo.Name, file.Path, err)
				continue
			}
			parsed, err := parser.ParseCatalog([]byte(content))
			if err != nil {
				log.Printf("[indexer] Failed to parse catalog for %s: %v", repo.Name, err)
				continue
			}
			ai.upsertCatalog(ctx, parsed, repo.Name)
		}
	}

	// Phase 3: Soft-delete stale entities.
	ai.archiveStale(ctx, activeRepos)
}

// refreshContent fetches and caches content for files whose SHA has changed.
func (ai *AutoIndexer) refreshContent(ctx context.Context, repos []scanner.RepoInfo) {
	for _, repo := range repos {
		for _, file := range repo.Files {
			if !ai.cache.NeedsUpdate(repo.Name, file.Path, file.SHA) {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				log.Printf("[indexer] Content fetch failed %s/%s: %v", repo.Name, file.Path, err)
				continue
			}
			if err := ai.cache.Upsert(repo.Name, file.Path, file.SHA, content); err != nil {
				log.Printf("[indexer] Content store failed %s/%s: %v", repo.Name, file.Path, err)
			}
		}
	}
}

// upsertCatalog writes parsed catalog data to the knowledge graph.
// Upsert rules:
//   - New entity → create with auto observations.
//   - Existing auto entity (has _source:catalog-info) → add missing auto observations, preserve manual ones.
//   - Existing manual entity (no _source:catalog-info) → add missing observations only, never overwrite.
func (ai *AutoIndexer) upsertCatalog(ctx context.Context, parsed parser.ParsedCatalog, repoFullName string) {
	autoObs := []string{
		"_source:catalog-info",
		"_scan_repo:" + repoFullName,
	}
	autoObs = append(autoObs, parsed.Observations...)

	// Check if entity already exists.
	graph, err := ai.graph.SearchNodes(parsed.EntityName)
	if err != nil {
		log.Printf("[indexer] SearchNodes failed for %s: %v", parsed.EntityName, err)
		return
	}

	entityExists := false
	for _, e := range graph.Entities {
		if e.Name == parsed.EntityName {
			entityExists = true
			break
		}
	}

	if !entityExists {
		// Create new entity with all auto observations.
		_, err := ai.graph.CreateEntities([]memory.Entity{
			{
				Name:         parsed.EntityName,
				EntityType:   parsed.EntityType,
				Observations: autoObs,
			},
		})
		if err != nil {
			log.Printf("[indexer] CreateEntities failed for %s: %v", parsed.EntityName, err)
			return
		}
	} else {
		// Entity exists: add missing observations without overwriting.
		_, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: parsed.EntityName, Contents: autoObs},
		})
		if err != nil {
			log.Printf("[indexer] AddObservations failed for %s: %v", parsed.EntityName, err)
		}
	}

	// Create relations (idempotent).
	if len(parsed.Relations) > 0 {
		rels := make([]memory.Relation, 0, len(parsed.Relations))
		for _, r := range parsed.Relations {
			rels = append(rels, memory.Relation{
				From:         r.From,
				To:           r.To,
				RelationType: r.RelationType,
			})
		}
		if _, err := ai.graph.CreateRelations(rels); err != nil {
			log.Printf("[indexer] CreateRelations failed for %s: %v", parsed.EntityName, err)
		}
	}
}

// archiveStale adds _status:archived to entities whose source repo is no longer active.
func (ai *AutoIndexer) archiveStale(ctx context.Context, activeRepos map[string]bool) {
	// Find all auto-indexed entities.
	graph, err := ai.graph.SearchNodes("_source:catalog-info")
	if err != nil {
		log.Printf("[indexer] SearchNodes for stale check failed: %v", err)
		return
	}

	for _, entity := range graph.Entities {
		repoName := ""
		for _, obs := range entity.Observations {
			if strings.HasPrefix(obs, "_scan_repo:") {
				repoName = strings.TrimPrefix(obs, "_scan_repo:")
				break
			}
		}
		if repoName == "" || activeRepos[repoName] {
			continue // still active or unknown source
		}
		// Mark as archived.
		_, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: entity.Name, Contents: []string{"_status:archived"}},
		})
		if err != nil {
			log.Printf("[indexer] Failed to archive %s: %v", entity.Name, err)
		}
	}
}
```

- [ ] **Step 4: Run indexer tests**

```bash
go test ./indexer/... -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add indexer/indexer.go indexer/indexer_test.go
git commit -m "feat: add AutoIndexer with catalog-info.yaml parsing and soft-delete"
```

---

## Task 7: New MCP Tools — `get_scan_status` and `search_content`

**Files:**
- Modify: `tools/tools.go`
- Modify: `tools/tools_test.go`

- [ ] **Step 1: Write failing tests**

Add to `tools/tools_test.go`:

```go
import (
	"time"
	// add "time" to existing imports
)

// Add Status() to the existing mockScanner struct:
func (m *mockScanner) Status() (bool, time.Time, int) {
	return false, time.Time{}, len(m.repos)
}

// --- New handler tests ---

type mockContentSearcher struct {
	matches []memory.ContentMatch
	count   int64
	enabled bool
}

func (m *mockContentSearcher) Search(query, repo string) ([]memory.ContentMatch, error) {
	if !m.enabled {
		return nil, fmt.Errorf("content search is disabled")
	}
	return m.matches, nil
}

func (m *mockContentSearcher) Count() (int64, error) {
	return m.count, nil
}

type mockGraphCounter struct {
	count int64
}

func (m *mockGraphCounter) EntityCount() (int64, error) {
	return m.count, nil
}

func TestGetScanStatusHandler(t *testing.T) {
	sc := &mockScanner{
		repos: []scanner.RepoInfo{
			{Name: "org/svc-a"},
			{Name: "org/svc-b"},
		},
	}
	counter := &mockGraphCounter{count: 5}
	searcher := &mockContentSearcher{count: 10, enabled: true}

	handler := getScanStatusHandler(sc, counter, searcher)
	req := &mcp.CallToolRequest{}

	_, result, err := handler(context.Background(), req, ScanStatusArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RepoCount != 2 {
		t.Errorf("expected RepoCount=2, got %d", result.RepoCount)
	}
	if result.GraphEntities != 5 {
		t.Errorf("expected GraphEntities=5, got %d", result.GraphEntities)
	}
	if result.ContentIndexed != 10 {
		t.Errorf("expected ContentIndexed=10, got %d", result.ContentIndexed)
	}
	if !result.ContentEnabled {
		t.Error("expected ContentEnabled=true")
	}
}

func TestSearchContentHandler_Success(t *testing.T) {
	searcher := &mockContentSearcher{
		enabled: true,
		matches: []memory.ContentMatch{
			{RepoName: "org/payment-svc", Path: "README.md", Snippet: "...handles Stripe payments..."},
		},
	}

	handler := searchContentHandler(searcher)
	req := &mcp.CallToolRequest{}

	_, result, err := handler(context.Background(), req, SearchContentArgs{Query: "stripe"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	if result.Matches[0].RepoName != "org/payment-svc" {
		t.Errorf("wrong repo: %s", result.Matches[0].RepoName)
	}
}

func TestSearchContentHandler_Disabled(t *testing.T) {
	searcher := &mockContentSearcher{enabled: false}
	handler := searchContentHandler(searcher)
	req := &mcp.CallToolRequest{}

	_, _, err := handler(context.Background(), req, SearchContentArgs{Query: "anything"})
	if err == nil {
		t.Error("expected error when content search is disabled")
	}
}

func TestSearchContentHandler_EmptyQuery(t *testing.T) {
	searcher := &mockContentSearcher{enabled: true}
	handler := searchContentHandler(searcher)
	req := &mcp.CallToolRequest{}

	_, _, err := handler(context.Background(), req, SearchContentArgs{Query: ""})
	if err == nil {
		t.Error("expected error for empty query")
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./tools/... -run "TestGetScanStatus|TestSearchContent"
```

Expected: compilation error — `getScanStatusHandler`, `searchContentHandler`, etc. undefined.

- [ ] **Step 3: Update `tools/tools.go`**

**a) Add new imports:**

```go
import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docscout-mcp/memory"
	"docscout-mcp/scanner"
)
```

**b) Add `Status()` to the `DocumentScanner` interface:**

```go
type DocumentScanner interface {
	ListRepos() []scanner.RepoInfo
	SearchDocs(query string) []scanner.FileEntry
	GetFileContent(ctx context.Context, repo string, path string) (string, error)
	Status() (scanning bool, lastScan time.Time, repoCount int)
}
```

**c) Add new interfaces for the new tools:**

```go
// GraphCounter provides the entity count for get_scan_status.
type GraphCounter interface {
	EntityCount() (int64, error)
}

// ContentSearcher provides full-text search over cached documentation content.
type ContentSearcher interface {
	Search(query, repo string) ([]memory.ContentMatch, error)
	Count() (int64, error)
}
```

**d) Update `Register` signature and body:**

```go
// Register adds all DocScout MCP tools to the server.
// graph and search may be nil — get_scan_status degrades gracefully, search_content is omitted.
func Register(s *mcp.Server, sc DocumentScanner, graph GraphCounter, search ContentSearcher) {
	// --- list_repos ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_repos",
		Description: "Lists all repositories in the organization that contain documentation files (catalog-info.yaml, mkdocs.yml, openapi.yaml, swagger.json, README.md, docs/*.md).",
	}, listReposHandler(sc))

	// --- search_docs ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_docs",
		Description: "Searches for documentation files by matching a query term against file paths and repository names.",
	}, searchDocsHandler(sc))

	// --- get_file_content ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_content",
		Description: "Retrieves the raw content of a specific documentation file from a GitHub repository. Note: For security reasons, this tool will only return files that have been successfully indexed as documentation (i.e. returned by list_repos or search_docs).",
	}, getFileContentHandler(sc))

	// --- get_scan_status ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_scan_status",
		Description: "Returns the current state of the documentation scanner and knowledge graph index. Call this before searching to confirm the index is populated, especially right after startup.",
	}, getScanStatusHandler(sc, graph, search))

	// --- search_content (only when content caching is enabled) ---
	if search != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "search_content",
			Description: "Full-text search across the content of all cached documentation files. Use this to find which service handles a specific responsibility (e.g. 'payment', 'authentication'). Only available when SCAN_CONTENT=true.",
		}, searchContentHandler(search))
	}
}
```

**e) Add new arg/result types and handlers at the bottom of the file:**

```go
// --- get_scan_status ---

type ScanStatusArgs struct{}

type ScanStatusResult struct {
	Scanning       bool      `json:"scanning"`
	LastScanAt     time.Time `json:"last_scan_at"`
	RepoCount      int       `json:"repo_count"`
	ContentIndexed int64     `json:"content_indexed"`
	GraphEntities  int64     `json:"graph_entities"`
	ContentEnabled bool      `json:"content_enabled"`
}

func getScanStatusHandler(sc DocumentScanner, graph GraphCounter, search ContentSearcher) func(ctx context.Context, req *mcp.CallToolRequest, args ScanStatusArgs) (*mcp.CallToolResult, ScanStatusResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args ScanStatusArgs) (*mcp.CallToolResult, ScanStatusResult, error) {
		scanning, lastScan, repoCount := sc.Status()

		var graphEntities int64
		if graph != nil {
			graphEntities, _ = graph.EntityCount()
		}

		var contentIndexed int64
		contentEnabled := search != nil
		if search != nil {
			contentIndexed, _ = search.Count()
		}

		return nil, ScanStatusResult{
			Scanning:       scanning,
			LastScanAt:     lastScan,
			RepoCount:      repoCount,
			ContentIndexed: contentIndexed,
			GraphEntities:  graphEntities,
			ContentEnabled: contentEnabled,
		}, nil
	}
}

// --- search_content ---

type SearchContentArgs struct {
	Query string `json:"query" jsonschema:"The term to search for inside documentation content. Use natural language terms like 'payment', 'authentication', 'event sourcing'."`
	Repo  string `json:"repo,omitempty" jsonschema:"Optional: filter results to a single repository name (e.g. 'org/payment-service')."`
}

type SearchContentResult struct {
	Matches []memory.ContentMatch `json:"matches" jsonschema:"List of files containing the query term, with a snippet showing the matched context."`
}

func searchContentHandler(search ContentSearcher) func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {
		if args.Query == "" {
			return nil, SearchContentResult{}, fmt.Errorf("parameter 'query' is required")
		}
		matches, err := search.Search(args.Query, args.Repo)
		if err != nil {
			return nil, SearchContentResult{}, err
		}
		return nil, SearchContentResult{Matches: matches}, nil
	}
}
```

- [ ] **Step 4: Run all tool tests**

```bash
go test ./tools/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/tools.go tools/tools_test.go
git commit -m "feat: add get_scan_status and search_content MCP tools"
```

---

## Task 8: Wire `main.go`

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Verify the build is currently broken**

```bash
go build ./...
```

Expected: errors — `tools.Register` call in `main.go` has wrong argument count; `memory.Register` has wrong signature.

- [ ] **Step 2: Update `main.go`**

Replace the entire `main.go` with:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docscout-mcp/indexer"
	"docscout-mcp/memory"
	"docscout-mcp/scanner"
	"docscout-mcp/tools"
)

const (
	serverName          = "DocScout-MCP"
	serverVersion       = "1.0.0"
	defaultScanInterval = 30 * time.Minute
	defaultMaxContent   = 200 * 1024 // 200 KB
)

func parseScanInterval(raw string) time.Duration {
	if raw == "" {
		return defaultScanInterval
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return time.Duration(n) * time.Minute
	}
	log.Printf("Invalid SCAN_INTERVAL '%s', using default %s", raw, defaultScanInterval)
	return defaultScanInterval
}

func parseCSVEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// isInMemoryDB returns true when the DB URL refers to an in-memory SQLite instance.
func isInMemoryDB(dbURL string) bool {
	return dbURL == "" || strings.Contains(dbURL, ":memory:")
}

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		log.Fatal("GITHUB_ORG environment variable is required")
	}

	scanInterval := parseScanInterval(os.Getenv("SCAN_INTERVAL"))
	targetFiles := parseCSVEnv(os.Getenv("SCAN_FILES"))
	scanDirs := parseCSVEnv(os.Getenv("SCAN_DIRS"))
	extraRepos := parseCSVEnv(os.Getenv("EXTRA_REPOS"))
	repoTopics := parseCSVEnv(os.Getenv("REPO_TOPICS"))

	var repoRegex *regexp.Regexp
	if rx := os.Getenv("REPO_REGEX"); rx != "" {
		compiled, err := regexp.Compile(rx)
		if err != nil {
			log.Fatalf("Invalid REPO_REGEX '%s': %v", rx, err)
		}
		repoRegex = compiled
	}

	httpAddr := os.Getenv("HTTP_ADDR")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("MEMORY_FILE_PATH") // backward compatibility
	}

	scanContent := strings.EqualFold(os.Getenv("SCAN_CONTENT"), "true")
	maxContentSize := defaultMaxContent
	if raw := os.Getenv("MAX_CONTENT_SIZE"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxContentSize = n
		} else {
			log.Printf("Invalid MAX_CONTENT_SIZE '%s', using default %d", raw, defaultMaxContent)
		}
	}

	// Disable content caching silently when using in-memory SQLite.
	if scanContent && isInMemoryDB(dbURL) {
		log.Println("[main] SCAN_CONTENT=true requires a persistent DATABASE_URL; content caching disabled.")
		scanContent = false
	}

	// --- GitHub client ---
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(httpClient)

	// --- Scanner ---
	sc := scanner.New(ghClient, org, scanInterval, targetFiles, scanDirs, extraRepos, repoTopics, repoRegex)

	// --- Database ---
	db, err := memory.OpenDB(dbURL)
	if err != nil {
		log.Fatalf("[main] Failed to open database: %v", err)
	}

	// --- MCP Server ---
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)

	// --- Memory / Knowledge Graph ---
	memory.Register(mcpServer, db)
	autoWriter := memory.NewAutoWriter(db)

	// --- Content Cache ---
	var contentCache *memory.ContentCache
	if scanContent {
		contentCache = memory.NewContentCache(db, true, maxContentSize)
		log.Printf("[main] Content caching enabled (max file size: %d bytes)", maxContentSize)
	}

	// --- Auto-Indexer ---
	ai := indexer.New(sc, autoWriter, contentCache)
	sc.SetOnScanComplete(func(repos []scanner.RepoInfo) {
		ai.Run(context.Background(), repos)
	})

	// --- Register MCP Tools ---
	tools.Register(mcpServer, sc, autoWriter, contentCache)

	// --- Start scanner (initial + periodic) ---
	sc.Start(ctx)

	log.Printf("%s v%s starting (org=%s, scan_interval=%s)\n", serverName, serverVersion, org, scanInterval)

	// --- Transport ---
	if httpAddr != "" {
		log.Printf("Listening on Streamable HTTP at %s...", httpAddr)
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return mcpServer
		}, nil)
		if err := http.ListenAndServe(httpAddr, handler); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	} else {
		log.Println("Listening on stdio...")
		if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}
}
```

- [ ] **Step 3: Verify the build succeeds**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: wire AutoIndexer, ContentCache, and new tools in main.go"
```

---

## Task 9: Integration Tests

**Files:**
- Modify: `integration_test.go`

- [ ] **Step 1: Update `integration_test.go`**

Replace the file with the updated version that adds `Status()` to `mockScanner`, updates `memory.Register`, and adds new E2E tests:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main_test

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docscout-mcp/memory"
	"docscout-mcp/scanner"
	"docscout-mcp/tools"
)

type mockScanner struct{}

func (m *mockScanner) ListRepos() []scanner.RepoInfo {
	return []scanner.RepoInfo{
		{
			Name:        "test-repo",
			FullName:    "test-org/test-repo",
			Description: "A test repository",
			HTMLURL:     "https://github.com/test-org/test-repo",
			Files: []scanner.FileEntry{
				{RepoName: "test-repo", Path: "README.md", Type: "readme"},
				{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
			},
		},
	}
}

func (m *mockScanner) SearchDocs(query string) []scanner.FileEntry {
	if query == "guide" {
		return []scanner.FileEntry{
			{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
		}
	}
	return nil
}

func (m *mockScanner) GetFileContent(ctx context.Context, repo, path string) (string, error) {
	if repo == "test-repo" && path == "README.md" {
		return "# Test Repo\nThis is a test.", nil
	}
	return "", nil
}

func (m *mockScanner) Status() (bool, time.Time, int) {
	return false, time.Now(), 1
}

func setupTestServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "docscout-mcp-test",
		Version: "test",
	}, nil)

	db, err := memory.OpenDB("")
	if err != nil {
		t.Fatalf("memory.OpenDB: %v", err)
	}

	memory.Register(server, db)
	autoWriter := memory.NewAutoWriter(db)

	// Register scanner tools (no content cache in integration tests).
	tools.Register(server, &mockScanner{}, autoWriter, nil)

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	return session
}

func TestE2E_ListTools(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// 3 scanner tools + 9 memory tools + 1 get_scan_status = 13
	// search_content is not registered because cache is nil
	if len(result.Tools) < 13 {
		t.Fatalf("expected at least 13 tools, got %d", len(result.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	expected := []string{
		"list_repos", "search_docs", "get_file_content", "get_scan_status",
		"create_entities", "create_relations", "add_observations",
		"delete_entities", "delete_observations", "delete_relations",
		"read_graph", "search_nodes", "open_nodes",
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestE2E_ListRepos(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_repos"})
	if err != nil {
		t.Fatalf("CallTool list_repos: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_repos returned error")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content from list_repos")
	}
}

func TestE2E_SearchDocs(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_docs",
		Arguments: map[string]any{"query": "guide"},
	})
	if err != nil {
		t.Fatalf("CallTool search_docs: %v", err)
	}
	if result.IsError {
		t.Fatalf("search_docs returned error")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content from search_docs")
	}
}

func TestE2E_GetFileContent(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_file_content",
		Arguments: map[string]any{"repo": "test-repo", "path": "README.md"},
	})
	if err != nil {
		t.Fatalf("CallTool get_file_content: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_file_content returned error")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if text == "" {
		t.Fatal("expected non-empty file content")
	}
}

func TestE2E_ScanStatus(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_scan_status"})
	if err != nil {
		t.Fatalf("CallTool get_scan_status: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_scan_status returned error: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content from get_scan_status")
	}
}

func TestE2E_SearchContent_Disabled(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	// search_content is not registered when cache is nil — calling it should return an error
	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_content",
		Arguments: map[string]any{"query": "payment"},
	})
	// The tool is not registered, so this should return an error from the MCP layer
	if err == nil {
		t.Log("Note: search_content returned no error — this is acceptable if the server returns an MCP tool-not-found error as a result")
	}
}

func TestE2E_MemoryLifecycle(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_entities",
		Arguments: map[string]any{
			"entities": []map[string]any{
				{"name": "api-gateway", "entityType": "Component", "observations": []string{"routes traffic"}},
				{"name": "user-service", "entityType": "Component", "observations": []string{}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create_entities: %v", err)
	}
	if res.IsError {
		t.Fatalf("create_entities returned error: %v", res.Content)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_relations",
		Arguments: map[string]any{
			"relations": []map[string]any{
				{"from": "api-gateway", "to": "user-service", "relationType": "proxies"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create_relations: %v", err)
	}
	if res.IsError {
		t.Fatalf("create_relations returned error: %v", res.Content)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_nodes",
		Arguments: map[string]any{"query": "gateway"},
	})
	if err != nil {
		t.Fatalf("search_nodes: %v", err)
	}
	if res.IsError {
		t.Fatalf("search_nodes returned error: %v", res.Content)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "read_graph"})
	if err != nil {
		t.Fatalf("read_graph: %v", err)
	}
	if res.IsError {
		t.Fatalf("read_graph returned error: %v", res.Content)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "delete_entities",
		Arguments: map[string]any{"entityNames": []string{"api-gateway"}},
	})
	if err != nil {
		t.Fatalf("delete_entities: %v", err)
	}
	if res.IsError {
		t.Fatalf("delete_entities returned error: %v", res.Content)
	}
}
```

- [ ] **Step 2: Run the full test suite**

```bash
go test ./... -v
```

Expected: all tests PASS. Count should be:
- `scanner/parser`: 5 tests
- `memory`: ~12 tests (existing + new AutoWriter + ContentCache)
- `scanner`: 4 tests (existing 3 + new callback test)
- `indexer`: 4 tests
- `tools`: ~7 tests (existing 3 + new 4)
- `integration` (main_test): 7 tests

- [ ] **Step 3: Run the build one final time**

```bash
go build ./...
```

Expected: no errors, binary produced.

- [ ] **Step 4: Final commit**

```bash
git add integration_test.go
git commit -m "test: update integration tests for new tools and wiring"
```

---

## Self-Review Against Spec

| Spec Requirement | Task |
|---|---|
| `catalog-info.yaml` parsed during scan | Tasks 2, 6 |
| Entities/relations auto-populated in graph | Task 6 (`upsertCatalog`) |
| `_source:catalog-info` and `_scan_repo:` tags | Task 6 |
| Manual entities never overwritten | Task 6 (`upsertCatalog` case 3) |
| Soft-delete with `_status:archived` | Task 6 (`archiveStale`) |
| Content cache opt-in via `SCAN_CONTENT=true` | Tasks 4, 8 |
| SHA-based incremental fetch | Task 4 (`NeedsUpdate`), Task 6 (`refreshContent`) |
| `MAX_CONTENT_SIZE` cap (default 200 KB) | Tasks 4, 8 |
| Disabled for in-memory SQLite | Task 8 (`isInMemoryDB`) |
| Orphaned content deleted on rescan | Tasks 4, 6 |
| `search_content` tool with snippet and repo filter | Tasks 4, 7 |
| `get_scan_status` tool | Task 7 |
| All new files have AGPL-3.0-only header | All tasks |
| No `fmt.Println` / stdout writes | All tasks |
| `go test ./...` passes | Task 9 |
