// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"time"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/leonancarvalho/docscout-mcp/scanner"
)

// DocumentScanner defines the interface for interacting with the documentation scanner.
type DocumentScanner interface {
	ListRepos() []scanner.RepoInfo
	SearchDocs(query string) []scanner.FileEntry
	GetFileContent(ctx context.Context, repo string, path string) (string, error)
	Status() (scanning bool, lastScan time.Time, repoCount int)
}

// GraphStore provides full access to the Knowledge Graph domain layer.
type GraphStore interface {
	CreateEntities(entities []memory.Entity) ([]memory.Entity, error)
	CreateRelations(relations []memory.Relation) ([]memory.Relation, error)
	AddObservations(observations []memory.Observation) ([]memory.Observation, error)
	DeleteEntities(entityNames []string) error
	DeleteObservations(deletions []memory.Observation) error
	DeleteRelations(relations []memory.Relation) error
	ReadGraph() (memory.KnowledgeGraph, error)
	SearchNodes(query string) (memory.KnowledgeGraph, error)
	OpenNodes(names []string) (memory.KnowledgeGraph, error)
	EntityCount() (int64, error)
	TraverseGraph(entity, relationType, direction string, maxDepth int) ([]memory.TraverseNode, error)
	GetIntegrationMap(ctx context.Context, service string, depth int) (memory.IntegrationMap, error)
}

// ContentSearcher provides full-text search over cached documentation content.
type ContentSearcher interface {
	Search(query, repo string) ([]memory.ContentMatch, error)
	Count() (int64, error)
	// SearchMode returns the active search backend: "fts5" (SQLite FTS5) or "like" (LIKE fallback).
	SearchMode() string
}
