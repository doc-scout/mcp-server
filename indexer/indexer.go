// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer

import (
	"context"
	"log"
	"strings"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/leonancarvalho/docscout-mcp/scanner"
	"github.com/leonancarvalho/docscout-mcp/scanner/parser"
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
//   - Existing entity → add missing observations only, never overwrite.
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
