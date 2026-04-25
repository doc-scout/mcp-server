// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/google/go-github/v60/github"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"gorm.io/gorm"

	adaptermcp "github.com/doc-scout/mcp-server/internal/adapter/mcp"
	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
	corecontent "github.com/doc-scout/mcp-server/internal/core/content"
	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
	"github.com/doc-scout/mcp-server/internal/infra/db"
	"github.com/doc-scout/mcp-server/internal/infra/embeddings"
	ghinfra "github.com/doc-scout/mcp-server/internal/infra/github"
	"github.com/doc-scout/mcp-server/internal/infra/github/parser"
	mcpparser "github.com/doc-scout/mcp-server/internal/infra/github/parser/mcp"
)

// Components holds all wired application dependencies.
type Components struct {
	DB           *gorm.DB
	Scanner      *ghinfra.Scanner
	Graph        *GraphAuditLogger
	AuditStore   coreaudit.AuditStore
	ContentCache corecontent.ContentRepository
	SemanticSrv  adaptermcp.SemanticSearch
	ToolMetrics  *adaptermcp.ToolMetrics
	DocMetrics   *adaptermcp.DocMetrics
	MCPServer    *mcp.Server
	Indexer      *AutoIndexer
	Cfg          Config
}

var serverVersion = "dev" // set at build time via -ldflags "-X main.serverVersion=<tag>"

// Wire builds and wires all application components from cfg.
func Wire(ctx context.Context, cfg Config) (*Components, error) {
	// --- GitHub client ---
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GitHubToken})
	httpClient := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(httpClient)

	// --- Parser Registry ---
	parser.Register(parser.GoModParser())
	parser.Register(parser.PackageJSONParser())
	parser.Register(parser.PomParser())
	parser.Register(parser.CodeownersParser())
	parser.Register(parser.CatalogParser())
	parser.Register(parser.AsyncAPIParser())
	parser.Register(parser.SpringKafkaParser())
	parser.Register(parser.OpenAPIParser())
	parser.Register(parser.ProtoParser())
	parser.Register(parser.K8sServiceParser())
	parser.Register(mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers()))

	// --- Scanner ---
	sc := ghinfra.New(ghClient, cfg.GitHubOrg, cfg.ScanInterval, cfg.TargetFiles, cfg.ScanDirs, cfg.InfraDirs, cfg.ExtraRepos, cfg.RepoTopics, cfg.RepoRegex, parser.Default)

	if cfg.RepoRegex != nil || len(cfg.RepoTopics) > 0 {
		slog.Warn("Repository filters are active. Entities from repos excluded by these filters will be marked _status:archived on the next scan.",
			"REPO_REGEX", cfg.RepoRegex,
			"REPO_TOPICS", cfg.RepoTopics)
	}

	// --- Database ---
	database, err := db.OpenDB(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// --- Graph Repository + Service ---
	graphRepo := db.NewGraphRepo(database)
	memorySrv := coregraph.NewMemoryService(graphRepo)

	// --- Audit Store ---
	var auditStore coreaudit.AuditStore
	if !isInMemoryDB(cfg.DatabaseURL) {
		as, err := db.NewAuditStore(database)
		if err != nil {
			return nil, err
		}
		auditStore = as
		slog.Info("Audit persistence enabled")
	} else {
		slog.Info("Audit persistence disabled (no persistent DATABASE_URL)")
	}

	// --- Agent identity capture ---
	var capturedClient atomic.Value
	capturedClient.Store("")

	// --- MCP Server ---
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "DocScout-MCP",
		Version: serverVersion,
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

	// --- Audit Logger decorator ---
	agentFn := func() string {
		client, _ := capturedClient.Load().(string)
		if cfg.AgentID != "" {
			return cfg.AgentID
		}
		if client != "" {
			return client
		}
		return "unknown"
	}
	auditedGraph := NewGraphAuditLogger(memorySrv, agentFn, auditStore)

	// --- Content Cache ---
	var contentCache corecontent.ContentRepository
	if cfg.ScanContent {
		contentCache = db.NewContentCache(database, true, cfg.MaxContentSize)
		slog.Info("Content caching enabled", "maxFileSize", cfg.MaxContentSize)
	}

	// --- Semantic Search ---
	embProvider := embeddings.NewProvider(cfg.EmbedConfig)
	var semanticSrv adaptermcp.SemanticSearch
	if embProvider != nil {
		embStore, err := embeddings.NewVectorStore(database)
		if err != nil {
			return nil, err
		}
		var docSrc embeddings.DocStore
		if ds, ok := contentCache.(embeddings.DocStore); ok {
			docSrc = ds
		}
		embIndexer := embeddings.NewIndexer(embProvider, embStore, docSrc, memorySrv)
		semanticSrv = embeddings.NewSemanticSearcher(embProvider, embStore, embIndexer, docSrc, memorySrv)
		slog.Info("[embeddings] Semantic search enabled", "provider", embProvider.ModelKey())
	} else {
		slog.Info("[embeddings] Semantic search disabled (no DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL)")
	}

	// --- Metrics ---
	toolMetrics := adaptermcp.NewToolMetrics()
	docMetrics := adaptermcp.NewDocMetrics()

	// --- Auto-Indexer ---
	ai := New(sc, auditedGraph, contentCache, parser.Default)

	return &Components{
		DB:           database,
		Scanner:      sc,
		Graph:        auditedGraph,
		AuditStore:   auditStore,
		ContentCache: contentCache,
		SemanticSrv:  semanticSrv,
		ToolMetrics:  toolMetrics,
		DocMetrics:   docMetrics,
		MCPServer:    mcpSrv,
		Indexer:      ai,
		Cfg:          cfg,
	}, nil
}
