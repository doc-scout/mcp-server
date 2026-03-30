// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
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
}

func getScanStatusHandler(sc DocumentScanner, graph GraphStore, search ContentSearcher) func(ctx context.Context, req *mcp.CallToolRequest, args ScanStatusArgs) (*mcp.CallToolResult, ScanStatusResult, error) {
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
