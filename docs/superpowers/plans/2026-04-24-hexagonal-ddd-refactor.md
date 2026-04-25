# Hexagonal/DDD Architecture Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the DocScout-MCP codebase from a flat package layout into an explicit Hexagonal/DDD architecture without rewriting any business logic.

**Architecture:** `core/` owns domain models and port interfaces; `infra/` implements outbound ports (DB, GitHub, embeddings); `adapter/` implements inbound ports (MCP tools, HTTP); `app/` wires everything and hosts the HTTP/stdio server; `cmd/docscout/` is the 20-line entrypoint. The single rule: `core/` never imports `infra/` or `adapter/`.

**Tech Stack:** Go 1.26, GORM (SQLite + Postgres), `modelcontextprotocol/go-sdk`, `google/go-github/v60`

---

## File Map

### Created (new files)
| New path | Source |
|---|---|
| `internal/core/graph/model.go` | Types from `memory/memory.go`, `memory/traverse.go`, `memory/pathfind.go`, `memory/integration.go` |
| `internal/core/graph/port.go` | New — interfaces |
| `internal/core/graph/service.go` | `memory/memory.go` — MemoryService, adapted |
| `internal/core/graph/export.go` | `memory/export.go` — pkg rename only |
| `internal/core/scan/model.go` | `scanner/scanner.go` — RepoInfo, FileEntry |
| `internal/core/scan/port.go` | New — interfaces |
| `internal/core/audit/model.go` | `memory/audit_store.go` — AuditEvent, AuditFilter, AuditSummary |
| `internal/core/audit/port.go` | New — interfaces |
| `internal/core/content/model.go` | `memory/content.go` — ContentMatch, DocRecord |
| `internal/core/content/port.go` | New — interfaces |
| `internal/infra/db/models.go` | `memory/memory.go` — private GORM structs |
| `internal/infra/db/migrate.go` | `memory/memory.go` — OpenDB + AutoMigrate |
| `internal/infra/db/graph_repo.go` | `memory/memory.go` + `memory/traverse.go` + `memory/pathfind.go` + `memory/integration.go` |
| `internal/infra/db/audit_repo.go` | `memory/audit_store.go` — DBAuditStore |
| `internal/infra/db/content_repo.go` | `memory/content.go` — ContentCache |
| `internal/infra/github/scanner.go` | `scanner/scanner.go` — imports updated |
| `internal/infra/github/retry.go` | `scanner/retry.go` — pkg rename only |
| `internal/infra/github/parser/*` | `scanner/parser/*` — pkg rename only |
| `internal/infra/embeddings/*` | `embeddings/*` — imports updated |
| `internal/adapter/mcp/ports.go` | `tools/ports.go` — types updated |
| `internal/adapter/mcp/validation.go` | `tools/graph_guard.go` — pkg rename |
| `internal/adapter/mcp/metrics.go` | `tools/metrics.go` — pkg rename |
| `internal/adapter/mcp/tools.go` | `tools/tools.go` — imports updated |
| `internal/adapter/mcp/*.go` | All tool handler files — imports updated |
| `internal/adapter/http/health.go` | `health/health.go` — pkg rename |
| `internal/adapter/http/webhook.go` | `webhook/webhook.go` — pkg rename |
| `internal/app/config.go` | New — env-var reading extracted from `main.go` |
| `internal/app/audit_logger.go` | `tools/audit.go` — imports updated |
| `internal/app/indexer.go` | `indexer/indexer.go` — imports updated |
| `internal/app/wire.go` | New — DI wiring extracted from `main.go` |
| `internal/app/server.go` | New — HTTP server extracted from `main.go` |
| `cmd/docscout/main.go` | New — 20-line entrypoint |

### Deleted (after build passes)
`main.go`, `memory/`, `tools/`, `scanner/`, `indexer/`, `embeddings/`, `health/`, `webhook/`

### Updated
`tests/testutils/utils.go` — import paths, `benchmark/cmd/main.go` — import paths

---

## Task 1: Create feature branch and scaffold directories

**Files:** none (git + mkdir)

- [ ] **Step 1: Create and switch to feature branch from main**

```bash
git checkout main
git pull origin main
git checkout -b refactor/hexagonal-ddd
```

Expected: `Switched to a new branch 'refactor/hexagonal-ddd'`

- [ ] **Step 2: Scaffold all new directories**

```bash
mkdir -p internal/core/graph \
         internal/core/scan \
         internal/core/audit \
         internal/core/content \
         internal/infra/db \
         internal/infra/github/parser/mcp \
         internal/infra/embeddings \
         internal/adapter/mcp \
         internal/adapter/http \
         internal/app \
         cmd/docscout
```

- [ ] **Step 3: Commit scaffold**

```bash
git add -A
git commit -m "chore: scaffold hexagonal directory structure"
```

---

## Task 2: `internal/core/graph/` — domain models

**Files:**
- Create: `internal/core/graph/model.go`
- Create: `internal/core/graph/export.go`

- [ ] **Step 1: Create `internal/core/graph/model.go`**

This file consolidates all domain types from `memory/memory.go`, `memory/traverse.go`, `memory/pathfind.go`, and `memory/integration.go`. No GORM imports.

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package graph

// Entity represents a knowledge graph node with observations.
type Entity struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
}

// Relation represents a directed edge between two entities.
type Relation struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
	Confidence   string `json:"confidence,omitempty"`
}

// Observation contains facts about an entity.
type Observation struct {
	EntityName   string   `json:"entityName"`
	Contents     []string `json:"contents"`
	Observations []string `json:"observations,omitempty"`
}

// KnowledgeGraph represents the complete graph structure.
type KnowledgeGraph struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}

// TraverseNode is a node reached during graph traversal.
type TraverseNode struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
	Distance     int      `json:"distance"`
	Path         []string `json:"path"`
}

// TraverseEdge is a directed edge discovered during graph traversal.
type TraverseEdge struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
	Confidence   string `json:"confidence,omitempty"`
}

// PathEdge is a single directed edge on the path between two entities.
type PathEdge struct {
	From         string `json:"from"`
	RelationType string `json:"relationType"`
	To           string `json:"to"`
	Confidence   string `json:"confidence,omitempty"`
}

// IntegrationEdge is a single integration relationship entry in an IntegrationMap.
type IntegrationEdge struct {
	Target     string `json:"target"`
	Schema     string `json:"schema,omitempty"`
	Version    string `json:"version,omitempty"`
	Paths      int    `json:"paths,omitempty"`
	Confidence string `json:"confidence"`
	SourceRepo string `json:"source_repo,omitempty"`
}

// IntegrationMap aggregates all integration relationships for a service.
type IntegrationMap struct {
	Service      string            `json:"service"`
	Publishes    []IntegrationEdge `json:"publishes"`
	Subscribes   []IntegrationEdge `json:"subscribes"`
	ExposesAPI   []IntegrationEdge `json:"exposes_api"`
	ProvidesGRPC []IntegrationEdge `json:"provides_grpc"`
	GRPCDeps     []IntegrationEdge `json:"grpc_deps"`
	Calls        []IntegrationEdge `json:"calls"`
	Coverage     string            `json:"graph_coverage"`
}
```

- [ ] **Step 2: Create `internal/core/graph/export.go`**

Copy `memory/export.go` verbatim, change only the package declaration:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package graph
```

Then copy the rest of `memory/export.go` unchanged (the `ExportGraph` function and all helpers reference only `KnowledgeGraph`, which is now in this package).

- [ ] **Step 3: Commit**

```bash
git add internal/core/graph/model.go internal/core/graph/export.go
git commit -m "feat(core/graph): add domain models and export utility"
```

---

## Task 3: `internal/core/graph/` — ports and service

**Files:**
- Create: `internal/core/graph/port.go`
- Create: `internal/core/graph/service.go`

- [ ] **Step 1: Create `internal/core/graph/port.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package graph

import "context"

// GraphRepository is the outbound port implemented by internal/infra/db.
type GraphRepository interface {
	CreateEntities([]Entity) ([]Entity, error)
	CreateRelations([]Relation) ([]Relation, error)
	AddObservations([]Observation) ([]Observation, error)
	DeleteEntities(entityNames []string) error
	DeleteObservations(deletions []Observation) error
	DeleteRelations(relations []Relation) error
	ReadGraph() (KnowledgeGraph, error)
	SearchNodes(query string) (KnowledgeGraph, error)
	SearchNodesFiltered(query string, includeArchived bool) (KnowledgeGraph, error)
	OpenNodes(names []string) (KnowledgeGraph, error)
	OpenNodesFiltered(names []string, includeArchived bool) (KnowledgeGraph, error)
	EntityCount() (int64, error)
	EntityTypeCounts() (map[string]int64, error)
	ListEntities(entityType string) (KnowledgeGraph, error)
	ListRelations(relationType, fromEntity string) ([]Relation, error)
	TraverseGraph(entity, relationType, direction string, maxDepth int) ([]TraverseNode, []TraverseEdge, error)
	GetIntegrationMap(ctx context.Context, service string, depth int) (IntegrationMap, error)
	FindPath(from, to string, maxDepth int) ([]PathEdge, error)
	UpdateEntity(oldName, newName, newType string) error
}

// GraphService is the inbound port called by internal/adapter/mcp and decorated by
// the audit logger in internal/app. Embedding GraphRepository allows decoration
// without re-listing every method.
type GraphService interface {
	GraphRepository
}
```

- [ ] **Step 2: Create `internal/core/graph/service.go`**

`MemoryService` is moved here unchanged except: `s store` field becomes `repo GraphRepository`, and the constructor accepts an interface.

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package graph

import "context"

// MemoryService implements GraphService by delegating to a GraphRepository.
type MemoryService struct {
	repo GraphRepository
}

// NewMemoryService creates a MemoryService backed by repo.
func NewMemoryService(repo GraphRepository) *MemoryService {
	return &MemoryService{repo: repo}
}

func (srv *MemoryService) CreateEntities(entities []Entity) ([]Entity, error) {
	return srv.repo.CreateEntities(entities)
}

func (srv *MemoryService) CreateRelations(relations []Relation) ([]Relation, error) {
	return srv.repo.CreateRelations(relations)
}

func (srv *MemoryService) AddObservations(obs []Observation) ([]Observation, error) {
	return srv.repo.AddObservations(obs)
}

func (srv *MemoryService) DeleteEntities(names []string) error {
	return srv.repo.DeleteEntities(names)
}

func (srv *MemoryService) DeleteObservations(deletions []Observation) error {
	return srv.repo.DeleteObservations(deletions)
}

func (srv *MemoryService) DeleteRelations(relations []Relation) error {
	return srv.repo.DeleteRelations(relations)
}

func (srv *MemoryService) ReadGraph() (KnowledgeGraph, error) {
	return srv.repo.ReadGraph()
}

func (srv *MemoryService) SearchNodes(query string) (KnowledgeGraph, error) {
	return srv.repo.SearchNodes(query)
}

func (srv *MemoryService) SearchNodesFiltered(query string, includeArchived bool) (KnowledgeGraph, error) {
	return srv.repo.SearchNodesFiltered(query, includeArchived)
}

func (srv *MemoryService) OpenNodes(names []string) (KnowledgeGraph, error) {
	return srv.repo.OpenNodes(names)
}

func (srv *MemoryService) OpenNodesFiltered(names []string, includeArchived bool) (KnowledgeGraph, error) {
	return srv.repo.OpenNodesFiltered(names, includeArchived)
}

func (srv *MemoryService) EntityCount() (int64, error) {
	return srv.repo.EntityCount()
}

func (srv *MemoryService) EntityTypeCounts() (map[string]int64, error) {
	return srv.repo.EntityTypeCounts()
}

func (srv *MemoryService) ListEntities(entityType string) (KnowledgeGraph, error) {
	return srv.repo.ListEntities(entityType)
}

func (srv *MemoryService) ListRelations(relationType, fromEntity string) ([]Relation, error) {
	return srv.repo.ListRelations(relationType, fromEntity)
}

func (srv *MemoryService) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]TraverseNode, []TraverseEdge, error) {
	return srv.repo.TraverseGraph(entity, relationType, direction, maxDepth)
}

func (srv *MemoryService) GetIntegrationMap(ctx context.Context, service string, depth int) (IntegrationMap, error) {
	return srv.repo.GetIntegrationMap(ctx, service, depth)
}

func (srv *MemoryService) FindPath(from, to string, maxDepth int) ([]PathEdge, error) {
	return srv.repo.FindPath(from, to, maxDepth)
}

func (srv *MemoryService) UpdateEntity(oldName, newName, newType string) error {
	return srv.repo.UpdateEntity(oldName, newName, newType)
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/core/graph/port.go internal/core/graph/service.go
git commit -m "feat(core/graph): add GraphRepository port and MemoryService"
```

---

## Task 4: `internal/core/scan/`, `internal/core/audit/`, `internal/core/content/`

**Files:**
- Create: `internal/core/scan/model.go`, `internal/core/scan/port.go`
- Create: `internal/core/audit/model.go`, `internal/core/audit/port.go`
- Create: `internal/core/content/model.go`, `internal/core/content/port.go`

- [ ] **Step 1: Create `internal/core/scan/model.go`**

Extract `RepoInfo` and `FileEntry` from `scanner/scanner.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package scan

// FileEntry represents an indexed documentation file.
type FileEntry struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Type     string `json:"type"`
}

// RepoInfo holds metadata about a repository that contains documentation.
type RepoInfo struct {
	Name        string      `json:"name"`
	FullName    string      `json:"full_name"`
	Description string      `json:"description"`
	HTMLURL     string      `json:"html_url"`
	Files       []FileEntry `json:"files"`
}
```

- [ ] **Step 2: Create `internal/core/scan/port.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package scan

import (
	"context"
	"time"
)

// ScannerGateway is the outbound port implemented by internal/infra/github.
type ScannerGateway interface {
	Start(ctx context.Context)
	SetOnScanComplete(func([]RepoInfo))
	Status() (scanning bool, lastScan time.Time, repoCount int)
	TriggerScan() bool
	TriggerRepoScan(ctx context.Context, owner, repo string)
}

// DocumentService is the inbound port called by internal/adapter/mcp tools.
type DocumentService interface {
	ListRepos() []RepoInfo
	SearchDocs(query string) []FileEntry
	GetFileContent(ctx context.Context, repo, path string) (string, error)
	Status() (scanning bool, lastScan time.Time, repoCount int)
	TriggerScan() bool
}
```

- [ ] **Step 3: Create `internal/core/audit/model.go`**

Extract types from `memory/audit_store.go`. Keep GORM struct tags (they are metadata strings — no gorm import needed in core):

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package audit

import "time"

// AuditEvent is persisted to the audit_events table.
type AuditEvent struct {
	ID        string    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	Agent     string
	Tool      string
	Operation string
	Targets   string // JSON array of entity/relation names
	Count     int
	Outcome   string // "ok" | "error"
	ErrorMsg  string
}

// AuditFilter constrains Query results.
type AuditFilter struct {
	Agent     string
	Tool      string
	Operation string
	Outcome   string
	Since     time.Time
	Limit     int
}

// AuditSummary is returned by AuditReader.Summary.
type AuditSummary struct {
	TotalMutations int
	ByAgent        map[string]int
	ByOperation    map[string]int
	ErrorRate      float64
	RiskyEvents    []AuditEvent
}
```

- [ ] **Step 4: Create `internal/core/audit/port.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package audit

import (
	"context"
	"time"
)

// AuditWriter is the write-only view used by the audit logger decorator.
type AuditWriter interface {
	Write(ctx context.Context, e AuditEvent) error
}

// AuditReader is the read-only view used by MCP tools and HTTP handlers.
type AuditReader interface {
	Query(ctx context.Context, f AuditFilter) ([]AuditEvent, int64, error)
	Summary(ctx context.Context, window time.Duration) (AuditSummary, error)
}

// AuditStore is the full port implemented by internal/infra/db.
type AuditStore interface {
	AuditWriter
	AuditReader
}
```

- [ ] **Step 5: Create `internal/core/content/model.go`**

Extract types from `memory/content.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package content

// ContentMatch is a search result from the content cache.
type ContentMatch struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	FileType string `json:"file_type,omitempty"`
	Snippet  string `json:"snippet"`
}

// DocRecord is a lightweight document record used by the semantic indexer.
type DocRecord struct {
	RepoName string
	Path     string
	DocID    string // "<RepoName>#<Path>"
	Content  string
}
```

- [ ] **Step 6: Create `internal/core/content/port.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package content

// ContentSearcher is the inbound port called by internal/adapter/mcp tools.
type ContentSearcher interface {
	Search(query, repo, fileType string) ([]ContentMatch, error)
	Count() (int64, error)
	SearchMode() string
}

// ContentRepository is the outbound port implemented by internal/infra/db.
type ContentRepository interface {
	ContentSearcher
	Upsert(repoName, path, sha, content, fileType string) error
	NeedsUpdate(repoName, path, sha string) bool
	DeleteOrphanedContent(activeRepos []string) error
	ListDocs(repoName string) ([]DocRecord, error)
}
```

- [ ] **Step 7: Commit**

```bash
git add internal/core/
git commit -m "feat(core): add scan, audit, content domain models and ports"
```

---

## Task 5: `internal/infra/db/` — GORM models, migrate, graph_repo

**Files:**
- Create: `internal/infra/db/models.go`
- Create: `internal/infra/db/migrate.go`
- Create: `internal/infra/db/graph_repo.go`

- [ ] **Step 1: Create `internal/infra/db/models.go`**

Private GORM models — extract from `memory/memory.go` and `memory/content.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package db

import "time"

type dbEntity struct {
	Name       string `gorm:"primaryKey"`
	EntityType string `gorm:"index"`
}

type dbRelation struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	FromEntity   string `gorm:"index;column:from_node"`
	ToEntity     string `gorm:"index;column:to_node"`
	RelationType string `gorm:"index"`
	Confidence   string `gorm:"default:authoritative"`
}

type dbObservation struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	EntityName string `gorm:"index;column:entity_name"`
	Content    string
}

type dbDocContent struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	RepoName  string    `gorm:"index;uniqueIndex:idx_repo_path"`
	Path      string    `gorm:"uniqueIndex:idx_repo_path"`
	SHA       string
	FileType  string    `gorm:"index"`
	Content   string    `gorm:"type:text"`
	IndexedAt time.Time
}
```

- [ ] **Step 2: Create `internal/infra/db/migrate.go`**

Extract `OpenDB` from `memory/memory.go`. References `dbEntity`, `dbRelation`, `dbObservation`, `dbDocContent` (private GORM models in this package), and `core/audit.AuditEvent` (has GORM tags, used for AutoMigrate):

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
)

var inMemoryCounter atomic.Int64

// OpenDB opens the database and runs AutoMigrate for all models.
// dbURL accepts: "sqlite://path.db", "postgres://...", a plain file path, or "" for in-memory SQLite.
func OpenDB(dbURL string) (*gorm.DB, error) {
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

	var (
		db  *gorm.DB
		err error
	)

	switch {
	case strings.HasPrefix(dbURL, "postgres://"), strings.HasPrefix(dbURL, "postgresql://"):
		db, err = gorm.Open(postgres.Open(dbURL), cfg)
	case strings.HasPrefix(dbURL, "sqlite://"):
		path := strings.TrimPrefix(dbURL, "sqlite://")
		db, err = gorm.Open(sqlite.Open(path), cfg)
	case dbURL == "":
		name := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared", inMemoryCounter.Add(1))
		db, err = gorm.Open(sqlite.Open(name), cfg)
	default:
		db, err = gorm.Open(sqlite.Open(dbURL), cfg)
	}

	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&dbEntity{}, &dbRelation{}, &dbObservation{}, &dbDocContent{},
		&coreaudit.AuditEvent{},
	); err != nil {
		return nil, err
	}

	return db, nil
}
```

- [ ] **Step 3: Create `internal/infra/db/graph_repo.go`**

This is the core GORM adapter. It consolidates:
- The private `store` struct methods from `memory/memory.go` (CRUD)
- `traverseGraph` / `queryNeighbours` / `edgeQuery` from `memory/traverse.go`
- `findPath` / `allEdgesFor` / `reconstructPath` from `memory/pathfind.go`
- `getIntegrationMap` from `memory/integration.go`

The package is `db`. All types referencing `memory.Entity` etc. now use `coregraph "github.com/doc-scout/mcp-server/internal/core/graph"`.

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

// GraphRepo is the GORM implementation of core/graph.GraphRepository.
type GraphRepo struct {
	db *gorm.DB
}

// NewGraphRepo creates a GraphRepo backed by db.
func NewGraphRepo(db *gorm.DB) *GraphRepo {
	return &GraphRepo{db: db}
}
```

Then copy all method bodies from the current `store` receiver in `memory/memory.go`, `memory/traverse.go`, `memory/pathfind.go`, and `memory/integration.go`, changing:
- Receiver: `(s store)` → `(r *GraphRepo)` and `s.db` → `r.db`
- Return types: `memory.Entity` → `coregraph.Entity`, `memory.Relation` → `coregraph.Relation`, etc.
- Private helpers `edgeRow`, `pathEdgeRow`, `pathParentInfo` stay private in this file

The method signatures must exactly match `coregraph.GraphRepository`:

```go
func (r *GraphRepo) CreateEntities(entities []coregraph.Entity) ([]coregraph.Entity, error)
func (r *GraphRepo) CreateRelations(relations []coregraph.Relation) ([]coregraph.Relation, error)
func (r *GraphRepo) AddObservations(observations []coregraph.Observation) ([]coregraph.Observation, error)
func (r *GraphRepo) DeleteEntities(entityNames []string) error
func (r *GraphRepo) DeleteObservations(deletions []coregraph.Observation) error
func (r *GraphRepo) DeleteRelations(relations []coregraph.Relation) error
func (r *GraphRepo) ReadGraph() (coregraph.KnowledgeGraph, error)
func (r *GraphRepo) SearchNodes(query string) (coregraph.KnowledgeGraph, error)
func (r *GraphRepo) SearchNodesFiltered(query string, includeArchived bool) (coregraph.KnowledgeGraph, error)
func (r *GraphRepo) OpenNodes(names []string) (coregraph.KnowledgeGraph, error)
func (r *GraphRepo) OpenNodesFiltered(names []string, includeArchived bool) (coregraph.KnowledgeGraph, error)
func (r *GraphRepo) EntityCount() (int64, error)
func (r *GraphRepo) EntityTypeCounts() (map[string]int64, error)
func (r *GraphRepo) ListEntities(entityType string) (coregraph.KnowledgeGraph, error)
func (r *GraphRepo) ListRelations(relationType, fromEntity string) ([]coregraph.Relation, error)
func (r *GraphRepo) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]coregraph.TraverseNode, []coregraph.TraverseEdge, error)
func (r *GraphRepo) GetIntegrationMap(ctx context.Context, service string, depth int) (coregraph.IntegrationMap, error)
func (r *GraphRepo) FindPath(from, to string, maxDepth int) ([]coregraph.PathEdge, error)
func (r *GraphRepo) UpdateEntity(oldName, newName, newType string) error
```

The internal helpers `loadGraph`, `createEntities`, `buildSubGraph`, `traverseGraph`, `queryNeighbours`, `edgeQuery`, `findPath`, `allEdgesFor`, `reconstructPath`, `getIntegrationMap`, `listRelations`, `updateEntity`, `searchNodesFiltered`, `openNodesFiltered`, `listEntities` are all moved as methods on `*GraphRepo`. Their bodies are **identical** to the current `memory/` implementations — only the receiver name and the type qualifiers change (`memory.Entity` → `coregraph.Entity`, etc.).

Private helper types used only within this file:
```go
type edgeRow struct {
	FromNode     string `gorm:"column:from_node"`
	ToNode       string `gorm:"column:to_node"`
	RelationType string `gorm:"column:relation_type"`
	Confidence   string `gorm:"column:confidence"`
}

type pathEdgeRow struct {
	FromNode     string `gorm:"column:from_node"`
	ToNode       string `gorm:"column:to_node"`
	RelationType string `gorm:"column:relation_type"`
	Confidence   string `gorm:"column:confidence"`
}

type pathParentInfo struct {
	parent string
	edge   coregraph.PathEdge
}
```

The `authoritativeSources` map from `memory/integration.go` moves here as a package-level var.

- [ ] **Step 4: Verify compilation of infra/db so far**

```bash
go build ./internal/infra/db/...
```

Expected: zero errors. If there are type mismatches, fix them before committing.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/db/models.go internal/infra/db/migrate.go internal/infra/db/graph_repo.go
git commit -m "feat(infra/db): add GraphRepo GORM adapter"
```

---

## Task 6: `internal/infra/db/` — audit and content repos

**Files:**
- Create: `internal/infra/db/audit_repo.go`
- Create: `internal/infra/db/content_repo.go`

- [ ] **Step 1: Create `internal/infra/db/audit_repo.go`**

Copy `memory/audit_store.go` entirely. Changes:
- `package memory` → `package db`
- Remove type definitions for `AuditEvent`, `AuditFilter`, `AuditSummary` (they are now in `core/audit`)
- Add import: `coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"`
- All references to `AuditEvent` → `coreaudit.AuditEvent`, `AuditFilter` → `coreaudit.AuditFilter`, `AuditSummary` → `coreaudit.AuditSummary`
- Remove the `AuditStore` interface definition (it's in `core/audit/port.go`)
- `DBAuditStore` now implements `coreaudit.AuditStore`
- `MarshalTargets` stays in this file (infra utility)

The `NewAuditStore` signature:
```go
func NewAuditStore(db *gorm.DB) (*DBAuditStore, error)
```

All method bodies are identical to `memory/audit_store.go`.

- [ ] **Step 2: Create `internal/infra/db/content_repo.go`**

Copy `memory/content.go` entirely. Changes:
- `package memory` → `package db`
- Remove `ContentMatch` and `DocRecord` type definitions (they are now in `core/content`)
- Add import: `corecontent "github.com/doc-scout/mcp-server/internal/core/content"`
- All references to `ContentMatch` → `corecontent.ContentMatch`, `DocRecord` → `corecontent.DocRecord`
- `dbDocContent` is already defined in `models.go` in this package — remove the duplicate definition from this file
- `ContentCache` now implements `corecontent.ContentRepository`
- All method bodies are identical to `memory/content.go`

The `NewContentCache` signature stays the same:
```go
func NewContentCache(db *gorm.DB, enabled bool, maxSize int) *ContentCache
```

- [ ] **Step 3: Verify full infra/db compilation**

```bash
go build ./internal/infra/db/...
```

Expected: zero errors.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/db/audit_repo.go internal/infra/db/content_repo.go
git commit -m "feat(infra/db): add DBAuditStore and ContentCache adapters"
```

---

## Task 7: `internal/infra/github/` — scanner and retry

**Files:**
- Create: `internal/infra/github/scanner.go`
- Create: `internal/infra/github/retry.go`

- [ ] **Step 1: Create `internal/infra/github/retry.go`**

Copy `scanner/retry.go` verbatim. Change only:
- `package scanner` → `package github`

- [ ] **Step 2: Create `internal/infra/github/scanner.go`**

Copy `scanner/scanner.go` entirely. Changes:
- `package scanner` → `package github`
- Remove `RepoInfo` and `FileEntry` type definitions (now in `core/scan`)
- Remove `DefaultTargetFiles` / `staticTargetFiles` / `DefaultScanDirs` / `DefaultInfraDirs` / `infraExtensions` — keep all these (they stay here as infra configuration)
- Add import: `corescan "github.com/doc-scout/mcp-server/internal/core/scan"`
- Replace `scanner/parser` import: `"github.com/doc-scout/mcp-server/scanner/parser"` → `"github.com/doc-scout/mcp-server/internal/infra/github/parser"`
- Replace all `RepoInfo` → `corescan.RepoInfo`, `FileEntry` → `corescan.FileEntry`
- The `Scanner` struct, `New`, `Start`, `TriggerScan`, `TriggerRepoScan`, `Status`, `ListRepos`, `SearchDocs`, `GetFileContent`, `SetOnScanComplete`, and all private scan methods move unchanged

The `Scanner` struct satisfies `corescan.ScannerGateway` and `corescan.DocumentService` implicitly (no explicit implementation needed in Go).

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/infra/github/...
```

Expected: zero errors (parsers not yet moved — this will fail until Task 8).

- [ ] **Step 4: Commit**

```bash
git add internal/infra/github/scanner.go internal/infra/github/retry.go
git commit -m "feat(infra/github): move Scanner to infra adapter"
```

---

## Task 8: `internal/infra/github/parser/` — move all parsers

**Files:** All files under `scanner/parser/` move to `internal/infra/github/parser/`

- [ ] **Step 1: Copy all parser files**

For each file in `scanner/parser/`:
- `asyncapi.go`, `catalog.go`, `codeowners.go`, `extension.go`, `gomod.go`, `k8sintegration.go`, `openapi.go`, `packagejson.go`, `pom.go`, `proto.go`, `registry.go`, `springkafka.go`

Change only:
- `package parser` stays the same (package name doesn't change, only the import path changes)

No other changes needed — parsers have no imports from `memory/` or `scanner/`.

- [ ] **Step 2: Copy `scanner/parser/mcp/` subdirectory**

Copy `scanner/parser/mcp/known_servers.go` and `scanner/parser/mcp/mcp_config_parser.go` to `internal/infra/github/parser/mcp/`.

Change only:
- `package mcp` stays the same

Import path for the parent parser package changes from `github.com/doc-scout/mcp-server/scanner/parser` → `github.com/doc-scout/mcp-server/internal/infra/github/parser`.

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/infra/github/...
```

Expected: zero errors.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/github/parser/
git commit -m "feat(infra/github): move file parsers to infra adapter"
```

---

## Task 9: `internal/infra/embeddings/` — move semantic search package

**Files:** All files under `embeddings/` move to `internal/infra/embeddings/`

- [ ] **Step 1: Copy all embeddings files**

Files: `indexer.go`, `ollama.go`, `openai.go`, `provider.go`, `searcher.go`, `similarity.go`, `store.go`

Changes for each file:
- `package embeddings` stays the same
- Replace `"github.com/doc-scout/mcp-server/memory"` with:
  - `coregraph "github.com/doc-scout/mcp-server/internal/core/graph"`
  - `corecontent "github.com/doc-scout/mcp-server/internal/core/content"`
- Replace all `memory.KnowledgeGraph` → `coregraph.KnowledgeGraph`
- Replace all `memory.DocRecord` → `corecontent.DocRecord`
- In `indexer.go`: the `DocStore` interface's `ListDocs` returns `[]corecontent.DocRecord`; `EntityOpener.OpenNodes` returns `coregraph.KnowledgeGraph`

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/infra/embeddings/...
```

Expected: zero errors.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/embeddings/
git commit -m "feat(infra/embeddings): move semantic search to infra adapter"
```

---

## Task 10: `internal/adapter/mcp/` — ports, validation, metrics, tools

**Files:**
- Create: `internal/adapter/mcp/ports.go`
- Create: `internal/adapter/mcp/validation.go`
- Create: `internal/adapter/mcp/metrics.go`
- Create: `internal/adapter/mcp/tools.go`

- [ ] **Step 1: Create `internal/adapter/mcp/ports.go`**

The interfaces become type aliases pointing to core ports, plus `SemanticSearch` which references `infra/embeddings` types:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"
	"time"

	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
	corecontent "github.com/doc-scout/mcp-server/internal/core/content"
	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
	corescan "github.com/doc-scout/mcp-server/internal/core/scan"
	"github.com/doc-scout/mcp-server/internal/infra/embeddings"
)

// DocumentScanner is the inbound port for scanner-related MCP tools.
type DocumentScanner = corescan.DocumentService

// GraphStore is the inbound port for graph-related MCP tools.
type GraphStore = coregraph.GraphService

// ContentSearcher is the inbound port for content search MCP tools.
type ContentSearcher = corecontent.ContentSearcher

// AuditReader is the read-only view of the audit store for MCP tools.
type AuditReader = coreaudit.AuditReader

// SemanticSearch gates the semantic search Plus feature.
// Pass nil to Register to disable semantic search entirely.
type SemanticSearch interface {
	Enabled() bool
	SearchDocs(ctx context.Context, query, repo string, topK int) ([]embeddings.DocResult, int, error)
	SearchEntities(ctx context.Context, query string, topK int) ([]embeddings.EntityResult, int, error)
	ScheduleIndexEntities(names []string)
	IndexDocs(ctx context.Context, repo string)
}
```

- [ ] **Step 2: Create `internal/adapter/mcp/validation.go`**

Copy `tools/graph_guard.go`. Changes:
- `package tools` → `package mcp`
- No import changes needed (no external imports)

- [ ] **Step 3: Create `internal/adapter/mcp/metrics.go`**

Copy `tools/metrics.go`. Changes:
- `package tools` → `package mcp`
- No import changes needed

- [ ] **Step 4: Create `internal/adapter/mcp/tools.go`**

Copy `tools/tools.go`. Changes:
- `package tools` → `package mcp`
- `"github.com/doc-scout/mcp-server/memory"` → `corecontent "github.com/doc-scout/mcp-server/internal/core/content"`
- In the `Register` function signature, `cache *memory.ContentCache` → `cache *db.ContentCache` — but this creates an import from adapter to infra. Instead, make it `cache corecontent.ContentRepository` to keep the dependency clean:

```go
func Register(
	s *mcp.Server,
	sc DocumentScanner,
	graph GraphStore,
	search ContentSearcher,
	semantic SemanticSearch,
	metrics *ToolMetrics,
	docMetrics *DocMetrics,
	cache corecontent.ContentRepository,
	readOnly bool,
	auditReader AuditReader,
)
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/mcp/ports.go internal/adapter/mcp/validation.go \
        internal/adapter/mcp/metrics.go internal/adapter/mcp/tools.go
git commit -m "feat(adapter/mcp): add ports, validation, metrics, tools registration"
```

---

## Task 11: `internal/adapter/mcp/` — tool handler files

**Files:** All `tools/*.go` handler files move to `internal/adapter/mcp/`

- [ ] **Step 1: Copy all tool handler files**

For each file in `tools/`: `add_observations.go`, `audit.go` (note: GraphAuditLogger is moving to `app/`, skip this), `create_entities.go`, `create_relations.go`, `delete_entities.go`, `delete_observations.go`, `delete_relations.go`, `discover_mcp_servers.go`, `export_graph.go`, `find_path.go`, `get_audit_summary.go`, `get_file_content.go`, `get_integration_map.go`, `get_scan_status.go`, `get_usage_stats.go`, `ingest_url.go`, `list_entities.go`, `list_relations.go`, `list_repos.go`, `open_nodes.go`, `query_audit_log.go`, `read_graph.go`, `search_content.go`, `search_docs.go`, `search_nodes.go`, `semantic_search.go`, `traverse_graph.go`, `trigger_scan.go`, `update_entity.go`

**Skip `tools/audit.go`** — `GraphAuditLogger` moves to `internal/app/audit_logger.go` in Task 12.

For each handler file, apply these import replacements:
- `package tools` → `package mcp`
- `"github.com/doc-scout/mcp-server/memory"` → `coregraph "github.com/doc-scout/mcp-server/internal/core/graph"` and/or `corecontent "github.com/doc-scout/mcp-server/internal/core/content"` and/or `coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"` (use whichever the file actually needs)
- `"github.com/doc-scout/mcp-server/scanner"` → `corescan "github.com/doc-scout/mcp-server/internal/core/scan"` (only in list_repos.go, search_docs.go, get_file_content.go, get_scan_status.go, trigger_scan.go)
- `"github.com/doc-scout/mcp-server/embeddings"` → `"github.com/doc-scout/mcp-server/internal/infra/embeddings"` (only in semantic_search.go)

Then rename all type references:
- `memory.Entity` → `coregraph.Entity`
- `memory.Relation` → `coregraph.Relation`
- `memory.Observation` → `coregraph.Observation`
- `memory.KnowledgeGraph` → `coregraph.KnowledgeGraph`
- `memory.TraverseNode` → `coregraph.TraverseNode`
- `memory.TraverseEdge` → `coregraph.TraverseEdge`
- `memory.PathEdge` → `coregraph.PathEdge`
- `memory.IntegrationMap` → `coregraph.IntegrationMap`
- `memory.AuditEvent` → `coreaudit.AuditEvent`
- `memory.AuditFilter` → `coreaudit.AuditFilter`
- `memory.AuditSummary` → `coreaudit.AuditSummary`
- `memory.ContentMatch` → `corecontent.ContentMatch`
- `memory.ContentCache` → `corecontent.ContentRepository` (where used as a type parameter)
- `scanner.RepoInfo` → `corescan.RepoInfo`
- `scanner.FileEntry` → `corescan.FileEntry`

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/adapter/mcp/...
```

Expected: zero errors. Fix any missed type references.

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/mcp/
git commit -m "feat(adapter/mcp): move all MCP tool handlers"
```

---

## Task 12: `internal/adapter/http/` — health and webhook

**Files:**
- Create: `internal/adapter/http/health.go`
- Create: `internal/adapter/http/webhook.go`

- [ ] **Step 1: Create `internal/adapter/http/health.go`**

Copy `health/health.go`. Changes:
- `package health` → `package http`

No import changes needed (uses only stdlib).

- [ ] **Step 2: Create `internal/adapter/http/webhook.go`**

Copy `webhook/webhook.go`. Changes:
- `package webhook` → `package http`

No import changes needed (uses only stdlib and `github.com/google/go-github/v60/github`).

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/http/
git commit -m "feat(adapter/http): move health and webhook handlers"
```

---

## Task 13: `internal/app/` — config, audit logger, indexer

**Files:**
- Create: `internal/app/config.go`
- Create: `internal/app/audit_logger.go`
- Create: `internal/app/indexer.go`

- [ ] **Step 1: Create `internal/app/config.go`**

Extract all env-var reading from `main.go` into a `Config` struct and `LoadConfig` function:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/doc-scout/mcp-server/internal/infra/embeddings"
)

const (
	defaultScanInterval = 30 * time.Minute
	defaultMaxContent   = 200 * 1024
)

// Config holds all runtime configuration read from environment variables.
type Config struct {
	GitHubToken    string
	GitHubOrg      string
	ScanInterval   time.Duration
	TargetFiles    []string
	ScanDirs       []string
	InfraDirs      []string
	ExtraRepos     []string
	RepoTopics     []string
	RepoRegex      *regexp.Regexp
	HTTPAddr       string
	DatabaseURL    string
	ScanContent    bool
	GraphReadOnly  bool
	MaxContentSize int
	AgentID        string
	WebhookSecret  string
	BearerToken    string
	EmbedConfig    embeddings.Config
}

// LoadConfig reads and validates all environment variables.
// Returns an error when required variables are missing or invalid.
func LoadConfig() (Config, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return Config{}, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		return Config{}, fmt.Errorf("GITHUB_ORG environment variable is required")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("MEMORY_FILE_PATH")
	}

	scanContent := strings.EqualFold(os.Getenv("SCAN_CONTENT"), "true")
	if scanContent && isInMemoryDB(dbURL) {
		slog.Error("SCAN_CONTENT=true requires a persistent DATABASE_URL. Content caching has been disabled.")
		scanContent = false
	}

	maxContentSize := defaultMaxContent
	if raw := os.Getenv("MAX_CONTENT_SIZE"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxContentSize = n
		} else {
			slog.Warn("Invalid MAX_CONTENT_SIZE, using default", "raw", raw)
		}
	}

	var repoRegex *regexp.Regexp
	if rx := os.Getenv("REPO_REGEX"); rx != "" {
		compiled, err := regexp.Compile(rx)
		if err != nil {
			return Config{}, fmt.Errorf("invalid REPO_REGEX %q: %w", rx, err)
		}
		repoRegex = compiled
	}

	graphReadOnly := strings.EqualFold(os.Getenv("GRAPH_READ_ONLY"), "true")
	if graphReadOnly {
		slog.Info("Graph read-only mode enabled")
	}

	return Config{
		GitHubToken:    token,
		GitHubOrg:      org,
		ScanInterval:   parseScanInterval(os.Getenv("SCAN_INTERVAL")),
		TargetFiles:    parseCSVEnv(os.Getenv("SCAN_FILES")),
		ScanDirs:       parseCSVEnv(os.Getenv("SCAN_DIRS")),
		InfraDirs:      parseCSVEnv(os.Getenv("SCAN_INFRA_DIRS")),
		ExtraRepos:     parseCSVEnv(os.Getenv("EXTRA_REPOS")),
		RepoTopics:     parseCSVEnv(os.Getenv("REPO_TOPICS")),
		RepoRegex:      repoRegex,
		HTTPAddr:       os.Getenv("HTTP_ADDR"),
		DatabaseURL:    dbURL,
		ScanContent:    scanContent,
		GraphReadOnly:  graphReadOnly,
		MaxContentSize: maxContentSize,
		AgentID:        os.Getenv("AGENT_ID"),
		WebhookSecret:  os.Getenv("GITHUB_WEBHOOK_SECRET"),
		BearerToken:    os.Getenv("MCP_HTTP_BEARER_TOKEN"),
		EmbedConfig:    embeddings.ConfigFromEnv(),
	}, nil
}

func isInMemoryDB(dbURL string) bool {
	return dbURL == "" || strings.Contains(dbURL, ":memory:")
}

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
	slog.Warn("Invalid SCAN_INTERVAL, using default", "raw", raw, "default", defaultScanInterval)
	return defaultScanInterval
}

func parseCSVEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var result []string
	for p := range strings.SplitSeq(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
```

Add `"fmt"` to imports.

- [ ] **Step 2: Create `internal/app/audit_logger.go`**

Copy `tools/audit.go`. Changes:
- `package tools` → `package app`
- `"github.com/doc-scout/mcp-server/memory"` → imports from core packages
- `GraphStore` (the wrapped type) → `coregraph.GraphService`
- `memory.AuditStore` → `coreaudit.AuditStore`
- All `memory.*` type references → corresponding `coregraph.*` or `coreaudit.*`

Resulting import block:
```go
import (
	"context"
	"fmt"
	"log/slog"

	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)
```

The `GraphAuditLogger` struct wraps `coregraph.GraphService` and uses `coreaudit.AuditStore`:
```go
type GraphAuditLogger struct {
	inner   coregraph.GraphService
	agentFn func() string
	store   coreaudit.AuditStore
}

func NewGraphAuditLogger(inner coregraph.GraphService, agentFn func() string, store coreaudit.AuditStore) *GraphAuditLogger {
	return &GraphAuditLogger{inner: inner, agentFn: agentFn, store: store}
}
```

All method bodies are identical to `tools/audit.go`, with `memory.*` replaced by `coregraph.*` / `coreaudit.*`.

`GraphAuditLogger` implements `coregraph.GraphService` (it must satisfy the full interface).

- [ ] **Step 3: Create `internal/app/indexer.go`**

Copy `indexer/indexer.go`. Changes:
- `package indexer` → `package app`
- `"github.com/doc-scout/mcp-server/memory"` → `coregraph "github.com/doc-scout/mcp-server/internal/core/graph"`; `corecontent "github.com/doc-scout/mcp-server/internal/core/content"`
- `"github.com/doc-scout/mcp-server/scanner"` → `corescan "github.com/doc-scout/mcp-server/internal/core/scan"`
- `"github.com/doc-scout/mcp-server/scanner/parser"` → `"github.com/doc-scout/mcp-server/internal/infra/github/parser"`
- `FileGetter` interface stays but uses `corescan.FileEntry` implicitly (method sig is `GetFileContent(ctx, repo, path string) (string, error)` — no types changed)
- `GraphWriter` interface uses `coregraph.*`:

```go
type GraphWriter interface {
	CreateEntities([]coregraph.Entity) ([]coregraph.Entity, error)
	CreateRelations([]coregraph.Relation) ([]coregraph.Relation, error)
	AddObservations([]coregraph.Observation) ([]coregraph.Observation, error)
	SearchNodes(query string) (coregraph.KnowledgeGraph, error)
	EntityCount() (int64, error)
}
```

- `AutoIndexer.cache` field type: `*memory.ContentCache` → `corecontent.ContentRepository`
- `AutoIndexer.sc` field type: `FileGetter` (unchanged, since it's already an interface)
- All `memory.Entity` → `coregraph.Entity`, `memory.Relation` → `coregraph.Relation`, etc.
- All `scanner.RepoInfo` → `corescan.RepoInfo`
- All method bodies are identical

- [ ] **Step 4: Commit**

```bash
git add internal/app/config.go internal/app/audit_logger.go internal/app/indexer.go
git commit -m "feat(app): add Config, GraphAuditLogger, AutoIndexer"
```

---

## Task 14: `internal/app/wire.go` and `internal/app/server.go`

**Files:**
- Create: `internal/app/wire.go`
- Create: `internal/app/server.go`

- [ ] **Step 1: Create `internal/app/wire.go`**

Extract the dependency-wiring section from `main.go`. `Wire` returns a ready-to-run `*App`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"cmp"
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v60/github"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	infra_db "github.com/doc-scout/mcp-server/internal/infra/db"
	infra_gh "github.com/doc-scout/mcp-server/internal/infra/github"
	ghparser "github.com/doc-scout/mcp-server/internal/infra/github/parser"
	mcpparser "github.com/doc-scout/mcp-server/internal/infra/github/parser/mcp"
	"github.com/doc-scout/mcp-server/internal/infra/embeddings"
	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
	corescan "github.com/doc-scout/mcp-server/internal/core/scan"
	adaptermcp "github.com/doc-scout/mcp-server/internal/adapter/mcp"
)

const (
	serverName    = "DocScout-MCP"
)

var ServerVersion = "dev"

// App holds the wired server ready to Run.
type App struct {
	cfg       Config
	mcpServer *mcp.Server
	scanner   corescan.ScannerGateway
}

// Wire builds the full dependency graph from cfg and returns a ready App.
func Wire(cfg Config) (*App, error) {
	// --- GitHub client ---
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GitHubToken})
	ghClient := github.NewClient(oauth2.NewClient(ctx, ts))

	// --- Parser Registry ---
	ghparser.Register(ghparser.GoModParser())
	ghparser.Register(ghparser.PackageJSONParser())
	ghparser.Register(ghparser.PomParser())
	ghparser.Register(ghparser.CodeownersParser())
	ghparser.Register(ghparser.CatalogParser())
	ghparser.Register(ghparser.AsyncAPIParser())
	ghparser.Register(ghparser.SpringKafkaParser())
	ghparser.Register(ghparser.OpenAPIParser())
	ghparser.Register(ghparser.ProtoParser())
	ghparser.Register(ghparser.K8sServiceParser())
	ghparser.Register(mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers()))

	// --- Database ---
	db, err := infra_db.OpenDB(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// --- Audit Store ---
	var auditStore coreaudit.AuditStore
	if !isInMemoryDB(cfg.DatabaseURL) {
		as, err := infra_db.NewAuditStore(db)
		if err != nil {
			return nil, fmt.Errorf("init audit store: %w", err)
		}
		auditStore = as
		slog.Info("Audit persistence enabled")
	} else {
		slog.Info("Audit persistence disabled (no persistent DATABASE_URL)")
	}

	// --- Graph ---
	graphRepo := infra_db.NewGraphRepo(db)
	memorySrv := coregraph.NewMemoryService(graphRepo)

	// --- Agent identity capture ---
	var capturedClient atomic.Value
	capturedClient.Store("")

	// --- MCP Server ---
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		InitializedHandler: func(_ context.Context, req *mcp.InitializedRequest) {
			if cfg.AgentID != "" {
				return
			}
			if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
				capturedClient.CompareAndSwap("", p.ClientInfo.Name)
			}
		},
	})

	agentFn := func() string {
		client, _ := capturedClient.Load().(string)
		return cmp.Or(cfg.AgentID, client, "unknown")
	}
	auditedGraph := NewGraphAuditLogger(memorySrv, agentFn, auditStore)

	// --- Content Cache ---
	var contentCache *infra_db.ContentCache
	if cfg.ScanContent {
		contentCache = infra_db.NewContentCache(db, true, cfg.MaxContentSize)
		slog.Info("Content caching enabled", "maxFileSize", cfg.MaxContentSize)
	}

	// --- Semantic Search ---
	embProvider := embeddings.NewProvider(cfg.EmbedConfig)
	var semanticSrv adaptermcp.SemanticSearch
	if embProvider != nil {
		embStore, err := embeddings.NewVectorStore(db)
		if err != nil {
			return nil, fmt.Errorf("create vector store: %w", err)
		}
		var docSrc embeddings.DocStore
		if contentCache != nil {
			docSrc = contentCache
		}
		embIndexer := embeddings.NewIndexer(embProvider, embStore, docSrc, memorySrv)
		semanticSrv = embeddings.NewSemanticSearcher(embProvider, embStore, embIndexer, docSrc, memorySrv)
		slog.Info("[embeddings] Semantic search enabled", "provider", embProvider.ModelKey())
	} else {
		slog.Info("[embeddings] Semantic search disabled")
	}

	// --- Scanner ---
	sc := infra_gh.New(ghClient, cfg.GitHubOrg, cfg.ScanInterval, cfg.TargetFiles, cfg.ScanDirs, cfg.InfraDirs, cfg.ExtraRepos, cfg.RepoTopics, cfg.RepoRegex, ghparser.Default)

	if cfg.RepoRegex != nil || len(cfg.RepoTopics) > 0 {
		slog.Warn("Repository filters are active. Excluded repos will be archived on next scan.",
			"REPO_REGEX", cfg.RepoRegex,
			"REPO_TOPICS", cfg.RepoTopics)
	}

	// --- Tool Metrics ---
	toolMetrics := adaptermcp.NewToolMetrics()
	docMetrics := adaptermcp.NewDocMetrics()

	// --- Auto-Indexer ---
	ai := NewAutoIndexer(sc, auditedGraph, contentCache, ghparser.Default)

	sc.SetOnScanComplete(func(repos []corescan.RepoInfo) {
		start := time.Now()
		slog.Info("[indexer] Auto-indexing started", "repos", len(repos))
		ai.Run(context.Background(), repos)
		slog.Info("[indexer] Auto-indexing complete", "duration", time.Since(start).String())

		var searcher adaptermcp.ContentSearcher
		if contentCache != nil {
			searcher = contentCache
		}
		adaptermcp.Register(mcpServer, sc, auditedGraph, searcher, semanticSrv, toolMetrics, docMetrics, contentCache, cfg.GraphReadOnly, auditStore)
		slog.Info("Triggered tools/list_changed notification")

		if semanticSrv != nil {
			for _, repo := range repos {
				go semanticSrv.IndexDocs(context.Background(), repo.FullName)
			}
		}
	})

	var searcher adaptermcp.ContentSearcher
	if contentCache != nil {
		searcher = contentCache
	}
	adaptermcp.Register(mcpServer, sc, auditedGraph, searcher, semanticSrv, toolMetrics, docMetrics, contentCache, cfg.GraphReadOnly, auditStore)

	return &App{
		cfg:       cfg,
		mcpServer: mcpServer,
		scanner:   sc,
	}, nil
}
```

Add missing imports (`fmt`, `coreaudit`). The `auditStore` variable uses `coreaudit.AuditStore`.

- [ ] **Step 2: Create `internal/app/server.go`**

Extract the HTTP server, stdio transport, and all HTTP route handlers from `main.go`:

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
	adapterhttp "github.com/doc-scout/mcp-server/internal/adapter/http"
	adaptermcp "github.com/doc-scout/mcp-server/internal/adapter/mcp"
)
```

`App.Run` replaces the transport branching in `main.go`. The HTTP route handlers for `/audit`, `/audit/summary`, `/metrics`, `/healthz`, `/webhook` move here verbatim from `main.go`, with these type updates:
- `memory.AuditFilter` → `coreaudit.AuditFilter`
- `memory.AuditEvent` → `coreaudit.AuditEvent`
- `health.Handler(...)` → `adapterhttp.Handler(...)`
- `webhook.Handler(...)` → `adapterhttp.Handler(...)` (different function, same package)

The `serverHealthProvider` struct and its `HealthStatus()` method also move to `server.go`:

```go
type serverHealthProvider struct {
	sc        corescan.ScannerGateway
	graph     adaptermcp.GraphStore
	startedAt time.Time
}

func (p *serverHealthProvider) HealthStatus() adapterhttp.Status {
	_, _, repoCount := p.sc.Status()
	status := "starting"
	if repoCount > 0 {
		status = "ok"
	}
	var entities int64
	if p.graph != nil {
		entities, _ = p.graph.EntityCount()
	}
	return adapterhttp.Status{
		Status:    status,
		StartedAt: p.startedAt,
		RepoCount: repoCount,
		Entities:  entities,
	}
}
```

`App.Run(ctx context.Context) error` starts the scanner and then branches on `cfg.HTTPAddr`:

```go
func (a *App) Run(ctx context.Context) error {
	a.scanner.Start(ctx)
	slog.Info("Server starting", "name", serverName, "version", ServerVersion,
		"org", a.cfg.GitHubOrg, "scan_interval", a.cfg.ScanInterval)

	if a.cfg.HTTPAddr != "" {
		return a.runHTTP(ctx)
	}
	return a.runStdio(ctx)
}
```

`runHTTP` and `runStdio` contain the corresponding blocks from `main.go` verbatim.

- [ ] **Step 3: Compile check**

```bash
go build ./internal/app/...
```

Expected: zero errors. Fix any missing imports or type mismatches.

- [ ] **Step 4: Commit**

```bash
git add internal/app/wire.go internal/app/server.go
git commit -m "feat(app): add Wire() and App.Run() — composition root and HTTP/stdio server"
```

---

## Task 15: `cmd/docscout/main.go` — new entrypoint

**Files:**
- Create: `cmd/docscout/main.go`

- [ ] **Step 1: Create `cmd/docscout/main.go`**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	benchmarkcmd "github.com/doc-scout/mcp-server/benchmark/cmd"
	"github.com/doc-scout/mcp-server/internal/app"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--benchmark" {
		os.Exit(benchmarkcmd.Run(os.Args[2:]))
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := app.LoadConfig()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	a, err := app.Wire(cfg)
	if err != nil {
		slog.Error("Failed to wire application", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := a.Run(ctx); err != nil {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build the new entrypoint**

```bash
go build ./cmd/docscout/...
```

Expected: zero errors. At this point both `main.go` (old) and `cmd/docscout/main.go` (new) exist — that is fine.

- [ ] **Step 3: Commit**

```bash
git add cmd/docscout/main.go
git commit -m "feat(cmd): add new cmd/docscout entrypoint"
```

---

## Task 16: Update tests and benchmark imports

**Files:**
- Modify: `tests/testutils/utils.go`
- Modify: `benchmark/cmd/main.go` (if it imports old packages)

- [ ] **Step 1: Check which test/benchmark files import old packages**

```bash
grep -r "doc-scout/mcp-server/memory\|doc-scout/mcp-server/tools\|doc-scout/mcp-server/scanner\|doc-scout/mcp-server/indexer\|doc-scout/mcp-server/embeddings\|doc-scout/mcp-server/health\|doc-scout/mcp-server/webhook" tests/ benchmark/
```

- [ ] **Step 2: Update `tests/testutils/utils.go`**

For each import found, apply the same replacements used in Tasks 5–14:
- `memory` → appropriate `internal/core/*` or `internal/infra/db` package
- `scanner` → `internal/core/scan` or `internal/infra/github`
- `tools` → `internal/adapter/mcp`

- [ ] **Step 3: Verify tests compile**

```bash
go build ./tests/...
```

Expected: zero errors.

- [ ] **Step 4: Commit**

```bash
git add tests/ benchmark/
git commit -m "chore(tests): update import paths to new hexagonal structure"
```

---

## Task 17: Delete old packages and final verification

**Files:** Delete `main.go`, `memory/`, `tools/`, `scanner/`, `indexer/`, `embeddings/`, `health/`, `webhook/`

- [ ] **Step 1: Delete old top-level packages**

```bash
git rm -r memory/ tools/ scanner/ indexer/ embeddings/ health/ webhook/ main.go
```

- [ ] **Step 2: Full build**

```bash
go build ./...
```

Expected: zero errors. If any file still imports a deleted package, fix it now.

- [ ] **Step 3: Run go vet**

```bash
go vet ./...
```

Expected: zero warnings.

- [ ] **Step 4: Run format check**

```bash
mise run format
```

Fix any import ordering issues reported by `gci`.

- [ ] **Step 5: Run existing tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 6: Verify dependency rule (core never imports infra)**

```bash
grep -r "doc-scout/mcp-server/internal/infra\|doc-scout/mcp-server/internal/adapter" internal/core/
```

Expected: no output (zero matches).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: remove old flat-package structure after hexagonal migration"
```

---

## Task 18: Open Pull Request

- [ ] **Step 1: Push branch**

```bash
git push -u origin refactor/hexagonal-ddd
```

- [ ] **Step 2: Open PR**

```bash
env -u GITHUB_TOKEN gh pr create \
  --base main \
  --head refactor/hexagonal-ddd \
  --title "refactor: migrate to hexagonal/DDD architecture" \
  --body "$(cat <<'EOF'
## Summary

- Restructures all packages into `internal/core/`, `internal/infra/`, `internal/adapter/`, `internal/app/`, and `cmd/docscout/` following Hexagonal/DDD principles
- Enforces dependency rule: `core/` never imports `infra/` or `adapter/`
- Splits 800-line `main.go` into `app/config.go`, `app/wire.go`, `app/server.go`, and `cmd/docscout/main.go`
- Zero logic changes — all algorithms, SQL, and MCP tool handlers are identical

## Spec

`docs/superpowers/specs/2026-04-24-hexagonal-ddd-design.md`

## Validation

- `go build ./...` ✅
- `go vet ./...` ✅
- `mise run format` ✅
- `go test ./...` ✅
- `grep -r "internal/infra" internal/core/` returns no results ✅
EOF
)"
```

- [ ] **Step 3: Share PR URL with user**

---

## Self-Review

**Spec coverage:**
- ✅ §2 Goals: directory restructure, dependency rule, main.go split, parser move — all covered in Tasks 1–17
- ✅ §5 Directory tree: every listed file has a corresponding task
- ✅ §6.1 MemoryService takes GraphRepository — Task 3
- ✅ §6.2 core/scan ports — Task 4
- ✅ §6.3 core/audit + core/content — Task 4
- ✅ §6.4 Config struct — Task 13
- ✅ §6.5 Wire() — Task 14
- ✅ §6.6 App.Run() — Task 14
- ✅ §6.7 GraphRepo — Task 5
- ✅ §6.8 adapter/mcp/ports.go type aliases — Task 10
- ✅ §7 Migration order — tasks ordered 2→3→4→5→6→7→8→9→10→11→12→13→14→15→16→17
- ✅ §9 Validation criteria — Task 17

**Placeholder scan:** No TBD, TODO, or vague instructions found.

**Type consistency:**
- `coregraph.GraphService` used consistently in `audit_logger.go` and `wire.go`
- `coreaudit.AuditStore` used consistently across `audit_repo.go`, `audit_logger.go`, `wire.go`
- `corecontent.ContentRepository` used in `tools.go` Register() signature and `wire.go`
- `corescan.RepoInfo` used consistently in scanner, indexer, wire
