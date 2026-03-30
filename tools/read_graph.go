// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func readGraphHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {
		g, err := graph.ReadGraph()
		if err != nil {
			return nil, memory.KnowledgeGraph{}, err
		}
		return nil, g, nil
	}
}
