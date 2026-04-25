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

// SemanticSearch gates the semantic search feature.
// Pass nil to Register to disable semantic search entirely.
type SemanticSearch interface {
	Enabled() bool
	SearchDocs(ctx context.Context, query, repo string, topK int) ([]embeddings.DocResult, int, error)
	SearchEntities(ctx context.Context, query string, topK int) ([]embeddings.EntityResult, int, error)
	ScheduleIndexEntities(names []string)
	IndexDocs(ctx context.Context, repo string)
}

// ensure unused import is used
var _ = time.Now
