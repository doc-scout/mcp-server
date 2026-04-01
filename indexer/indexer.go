// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer

import (
	"context"
	"log/slog"
	"slices"
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

	// Phase 2b: Auto-graph from go.mod — infer service identity and direct dependencies
	// without requiring a Backstage catalog-info.yaml.
	slog.Info("[indexer] Phase 2b: parsing go.mod files", "repos", len(repos))
	for _, repo := range repos {
		for _, file := range repo.Files {
			if file.Type != "gomod" {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				slog.Warn("[indexer] Failed to fetch go.mod", "repo", repo.Name, "error", err)
				continue
			}
			parsed, err := parser.ParseGoMod([]byte(content))
			if err != nil {
				slog.Warn("[indexer] Failed to parse go.mod", "repo", repo.Name, "error", err)
				continue
			}
			ai.upsertGoMod(ctx, parsed, repo.Name)
		}
	}

	// Phase 2c: Auto-graph from package.json — infer Node.js service identity and dependencies.
	slog.Info("[indexer] Phase 2c: parsing package.json files", "repos", len(repos))
	for _, repo := range repos {
		for _, file := range repo.Files {
			if file.Type != "packagejson" {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				slog.Warn("[indexer] Failed to fetch package.json", "repo", repo.Name, "error", err)
				continue
			}
			parsed, err := parser.ParsePackageJSON([]byte(content))
			if err != nil {
				slog.Warn("[indexer] Failed to parse package.json", "repo", repo.Name, "error", err)
				continue
			}
			ai.upsertPackageJSON(ctx, parsed, repo.Name)
		}
	}

	// Phase 2d: Auto-graph from pom.xml — infer Java/Maven service identity and dependencies.
	slog.Info("[indexer] Phase 2d: parsing pom.xml files", "repos", len(repos))
	for _, repo := range repos {
		for _, file := range repo.Files {
			if file.Type != "pomxml" {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				slog.Warn("[indexer] Failed to fetch pom.xml", "repo", repo.Name, "error", err)
				continue
			}
			parsed, err := parser.ParsePom([]byte(content))
			if err != nil {
				slog.Warn("[indexer] Failed to parse pom.xml", "repo", repo.Name, "error", err)
				continue
			}
			ai.upsertPom(ctx, parsed, repo.Name)
		}
	}

	// Phase 2e: Auto-graph from CODEOWNERS — create team/person entities and owns relations.
	slog.Info("[indexer] Phase 2e: parsing CODEOWNERS files", "repos", len(repos))
	for _, repo := range repos {
		for _, file := range repo.Files {
			if file.Type != "codeowners" {
				continue
			}
			content, err := ai.sc.GetFileContent(ctx, repo.Name, file.Path)
			if err != nil {
				slog.Warn("[indexer] Failed to fetch CODEOWNERS", "repo", repo.Name, "path", file.Path, "error", err)
				continue
			}
			parsed := parser.ParseCodeowners([]byte(content))
			ai.upsertCodeowners(ctx, parsed, repo.Name)
			// Only process the first CODEOWNERS found per repo to avoid duplicate relations.
			break
		}
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

	entityExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {
		return e.Name == parsed.EntityName
	})

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

// upsertGoMod writes go.mod data to the knowledge graph: a Service entity for the module
// and depends_on relations to each direct dependency.
func (ai *AutoIndexer) upsertGoMod(ctx context.Context, parsed parser.ParsedGoMod, repoFullName string) {
	autoObs := []string{
		"_source:go.mod",
		"_scan_repo:" + repoFullName,
		"go_module:" + parsed.ModulePath,
	}
	if parsed.GoVersion != "" {
		autoObs = append(autoObs, "go_version:"+parsed.GoVersion)
	}

	graph, err := ai.graph.SearchNodes(parsed.EntityName)
	if err != nil {
		slog.Error("[indexer] SearchNodes failed", "entity", parsed.EntityName, "error", err)
		return
	}

	entityExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {
		return e.Name == parsed.EntityName
	})

	if !entityExists {
		if _, err := ai.graph.CreateEntities([]memory.Entity{
			{Name: parsed.EntityName, EntityType: "service", Observations: autoObs},
		}); err != nil {
			slog.Error("[indexer] CreateEntities failed", "entity", parsed.EntityName, "error", err)
			return
		}
	} else {
		if _, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: parsed.EntityName, Contents: autoObs},
		}); err != nil {
			slog.Error("[indexer] AddObservations failed", "entity", parsed.EntityName, "error", err)
		}
	}

	if len(parsed.DirectDeps) == 0 {
		return
	}

	rels := make([]memory.Relation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		depName := moduleEntityName(dep)
		rels = append(rels, memory.Relation{
			From:         parsed.EntityName,
			To:           depName,
			RelationType: "depends_on",
		})
	}
	if _, err := ai.graph.CreateRelations(rels); err != nil {
		slog.Error("[indexer] CreateRelations failed", "entity", parsed.EntityName, "error", err)
	}
}

// upsertPackageJSON writes package.json data to the knowledge graph: a service entity
// and depends_on relations to each entry in "dependencies" (not devDependencies).
func (ai *AutoIndexer) upsertPackageJSON(ctx context.Context, parsed parser.ParsedPackageJSON, repoFullName string) {
	autoObs := []string{
		"_source:package.json",
		"_scan_repo:" + repoFullName,
		"npm_package:" + parsed.Name,
	}
	if parsed.Version != "" {
		autoObs = append(autoObs, "version:"+parsed.Version)
	}

	graph, err := ai.graph.SearchNodes(parsed.EntityName)
	if err != nil {
		slog.Error("[indexer] SearchNodes failed", "entity", parsed.EntityName, "error", err)
		return
	}

	entityExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {
		return e.Name == parsed.EntityName
	})

	if !entityExists {
		if _, err := ai.graph.CreateEntities([]memory.Entity{
			{Name: parsed.EntityName, EntityType: "service", Observations: autoObs},
		}); err != nil {
			slog.Error("[indexer] CreateEntities failed", "entity", parsed.EntityName, "error", err)
			return
		}
	} else {
		if _, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: parsed.EntityName, Contents: autoObs},
		}); err != nil {
			slog.Error("[indexer] AddObservations failed", "entity", parsed.EntityName, "error", err)
		}
	}

	if len(parsed.DirectDeps) == 0 {
		return
	}

	rels := make([]memory.Relation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		// Use parser's entity name normalizer so "@scope/pkg" → "pkg".
		depName := parser.PackageEntityName(dep)
		rels = append(rels, memory.Relation{
			From:         parsed.EntityName,
			To:           depName,
			RelationType: "depends_on",
		})
	}
	if _, err := ai.graph.CreateRelations(rels); err != nil {
		slog.Error("[indexer] CreateRelations failed", "entity", parsed.EntityName, "error", err)
	}
}

// upsertPom writes pom.xml data to the knowledge graph: a service entity for the Maven
// artifact and depends_on relations to each compile/runtime-scope dependency.
func (ai *AutoIndexer) upsertPom(ctx context.Context, parsed parser.ParsedPom, repoFullName string) {
	autoObs := []string{
		"_source:pom.xml",
		"_scan_repo:" + repoFullName,
		"maven_artifact:" + parsed.GroupID + ":" + parsed.ArtifactID,
	}
	if parsed.GroupID != "" {
		autoObs = append(autoObs, "java_group:"+parsed.GroupID)
	}
	if parsed.Version != "" {
		autoObs = append(autoObs, "version:"+parsed.Version)
	}

	graph, err := ai.graph.SearchNodes(parsed.EntityName)
	if err != nil {
		slog.Error("[indexer] SearchNodes failed", "entity", parsed.EntityName, "error", err)
		return
	}

	entityExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {
		return e.Name == parsed.EntityName
	})

	if !entityExists {
		if _, err := ai.graph.CreateEntities([]memory.Entity{
			{Name: parsed.EntityName, EntityType: "service", Observations: autoObs},
		}); err != nil {
			slog.Error("[indexer] CreateEntities failed", "entity", parsed.EntityName, "error", err)
			return
		}
	} else {
		if _, err := ai.graph.AddObservations([]memory.Observation{
			{EntityName: parsed.EntityName, Contents: autoObs},
		}); err != nil {
			slog.Error("[indexer] AddObservations failed", "entity", parsed.EntityName, "error", err)
		}
	}

	if len(parsed.DirectDeps) == 0 {
		return
	}

	rels := make([]memory.Relation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		rels = append(rels, memory.Relation{
			From:         parsed.EntityName,
			To:           dep,
			RelationType: "depends_on",
		})
	}
	if _, err := ai.graph.CreateRelations(rels); err != nil {
		slog.Error("[indexer] CreateRelations failed", "entity", parsed.EntityName, "error", err)
	}
}

// upsertCodeowners creates team/person entities for each unique owner found in CODEOWNERS
// and adds owns relations from each owner entity to the repository's service entity.
func (ai *AutoIndexer) upsertCodeowners(ctx context.Context, parsed parser.ParsedCodeowners, repoFullName string) {
	if len(parsed.UniqueOwners) == 0 {
		return
	}

	// Derive the target service name from the repo full name (e.g. "myorg/my-service" → "my-service").
	repoServiceName := repoFullName
	if _, name, ok := strings.Cut(repoFullName, "/"); ok {
		repoServiceName = name
	}

	var ownsRels []memory.Relation

	for _, owner := range parsed.UniqueOwners {
		autoObs := []string{
			"_source:CODEOWNERS",
			"_scan_repo:" + repoFullName,
			"github_handle:" + owner.Raw,
		}

		graph, err := ai.graph.SearchNodes(owner.EntityName)
		if err != nil {
			slog.Error("[indexer] SearchNodes failed", "entity", owner.EntityName, "error", err)
			continue
		}

		entityExists := slices.ContainsFunc(graph.Entities, func(e memory.Entity) bool {
			return e.Name == owner.EntityName
		})

		if !entityExists {
			if _, err := ai.graph.CreateEntities([]memory.Entity{
				{Name: owner.EntityName, EntityType: owner.EntityType, Observations: autoObs},
			}); err != nil {
				slog.Error("[indexer] CreateEntities failed", "entity", owner.EntityName, "error", err)
				continue
			}
		} else {
			if _, err := ai.graph.AddObservations([]memory.Observation{
				{EntityName: owner.EntityName, Contents: autoObs},
			}); err != nil {
				slog.Error("[indexer] AddObservations failed", "entity", owner.EntityName, "error", err)
			}
		}

		ownsRels = append(ownsRels, memory.Relation{
			From:         owner.EntityName,
			To:           repoServiceName,
			RelationType: "owns",
		})
	}

	if len(ownsRels) > 0 {
		if _, err := ai.graph.CreateRelations(ownsRels); err != nil {
			slog.Error("[indexer] CreateRelations failed for CODEOWNERS owns", "repo", repoFullName, "error", err)
		}
	}
}

// moduleEntityName extracts the last path segment from a Go module path.
// Duplicates parser.moduleEntityName to avoid exposing an internal parser helper.
func moduleEntityName(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	return parts[len(parts)-1]
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
			if name, ok := strings.CutPrefix(obs, "_scan_repo:"); ok {
				repoName = name
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
