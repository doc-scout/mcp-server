// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer

import (
	"context"
	"log/slog"
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
		slog.Info("[indexer] Phase 1: refreshing content cache", "repos", len(repos))
		ai.refreshContent(ctx, repos)
		activeRepoList := make([]string, 0, len(activeRepos))
		for name := range activeRepos {
			activeRepoList = append(activeRepoList, name)
		}
		if err := ai.cache.DeleteOrphanedContent(activeRepoList); err != nil {
			slog.Error("[indexer] Failed to delete orphaned content", "error", err)
		}
	}

	// Phase 2: Auto-graph from catalog-info.yaml.
	slog.Info("[indexer] Phase 2: parsing catalog-info.yaml files", "repos", len(repos))
	var fetchFailures, parseFailures int
	for _, repo := range repos {
		for _, file := range repo.Files {
			if file.Type != "catalog-info" {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				fetchFailures++
				slog.Warn("[indexer] Failed to fetch catalog file", "repo", repo.Name, "path", file.Path, "error", err)
				continue
			}
			parsed, err := parser.ParseCatalog([]byte(content))
			if err != nil {
				parseFailures++
				slog.Warn("[indexer] Failed to parse catalog", "repo", repo.Name, "error", err)
				continue
			}
			ai.upsertCatalog(ctx, parsed, repo.Name)
		}
	}
	if fetchFailures > 0 || parseFailures > 0 {
		slog.Warn("[indexer] Phase 2 completed with errors", "fetch_failures", fetchFailures, "parse_failures", parseFailures)
	}

	// Phase 3: Soft-delete stale entities.
	slog.Info("[indexer] Phase 3: archiving stale entities")
	ai.archiveStale(ctx, activeRepos)

	slog.Info("[indexer] Indexing complete", "active_repos", len(repos))
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
				slog.Warn("[indexer] Content fetch failed", "repo", repo.Name, "path", file.Path, "error", err)
				continue
			}
			if err := ai.cache.Upsert(repo.Name, file.Path, file.SHA, content); err != nil {
				slog.Warn("[indexer] Content store failed", "repo", repo.Name, "path", file.Path, "error", err)
			}
		}
	}
}

// upsertCatalog writes parsed catalog data to the knowledge graph.
// Upsert rules:
//   - New entity → create with auto observations.
//   - Existing entity → add missing observations only, never overwrite.
//
// Note: between SearchNodes and CreateEntities, a concurrent scan could create the
// same entity. The store's COUNT(*) deduplication handles this gracefully.
func (ai *AutoIndexer) upsertCatalog(ctx context.Context, parsed parser.ParsedCatalog, repoFullName string) {
	autoObs := []string{
		"_source:catalog-info",
		"_scan_repo:" + repoFullName,
	}
	autoObs = append(autoObs, parsed.Observations...)

	graph, err := ai.graph.SearchNodes(parsed.EntityName)
	if err != nil {
		slog.Error("[indexer] SearchNodes failed", "entity", parsed.EntityName, "error", err)
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
		_, err := ai.graph.CreateEntities([]memory.Entity{
			{
				Name:         parsed.EntityName,
				EntityType:   parsed.EntityType,
				Observations: autoObs,
			},
		})
		if err != nil {
			slog.Error("[indexer] CreateEntities failed", "entity", parsed.EntityName, "error", err)
			return
		}
	} else {
		_, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: parsed.EntityName, Contents: autoObs},
		})
		if err != nil {
			slog.Error("[indexer] AddObservations failed", "entity", parsed.EntityName, "error", err)
		}
	}

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
			slog.Error("[indexer] CreateRelations failed", "entity", parsed.EntityName, "error", err)
		}
	}
}

// archiveStale adds _status:archived to entities whose source repo is no longer active.
func (ai *AutoIndexer) archiveStale(ctx context.Context, activeRepos map[string]bool) {
	graph, err := ai.graph.SearchNodes("_source:catalog-info")
	if err != nil {
		slog.Error("[indexer] SearchNodes for stale check failed", "error", err)
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
			continue
		}
		_, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: entity.Name, Contents: []string{"_status:archived"}},
		})
		if err != nil {
			slog.Error("[indexer] Failed to archive entity", "entity", entity.Name, "error", err)
		}
	}
}
