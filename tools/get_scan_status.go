// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ScanStatusArgs struct{}

type ScanStatusResult struct {
	Scanning       bool      `json:"scanning"`
	LastScanAt     time.Time `json:"last_scan_at"`
	RepoCount      int       `json:"repo_count"`
	ContentIndexed int64     `json:"content_indexed"`
	GraphEntities  int64     `json:"graph_entities"`
	ContentEnabled bool      `json:"content_enabled"`
	// SearchMode is "fts5" when SQLite FTS5 full-text search is active, "like" for LIKE fallback, "" when disabled.
	SearchMode string `json:"search_mode,omitempty"`
}

func getScanStatusHandler(sc DocumentScanner, graph GraphStore, search ContentSearcher) func(ctx context.Context, req *mcp.CallToolRequest, args ScanStatusArgs) (*mcp.CallToolResult, ScanStatusResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args ScanStatusArgs) (*mcp.CallToolResult, ScanStatusResult, error) {
		scanning, lastScan, repoCount := sc.Status()

		var graphEntities int64
		if graph != nil {
			var err error
			graphEntities, err = graph.EntityCount()
			if err != nil {
				slog.Warn("[scan_status] EntityCount failed", "error", err)
			}
		}

		var contentIndexed int64
		var searchMode string
		contentEnabled := search != nil
		if search != nil {
			var err error
			contentIndexed, err = search.Count()
			if err != nil {
				slog.Warn("[scan_status] content Count failed", "error", err)
			}
			searchMode = search.SearchMode()
		}

		return nil, ScanStatusResult{
			Scanning:       scanning,
			LastScanAt:     lastScan,
			RepoCount:      repoCount,
			ContentIndexed: contentIndexed,
			GraphEntities:  graphEntities,
			ContentEnabled: contentEnabled,
			SearchMode:     searchMode,
		}, nil
	}
}
