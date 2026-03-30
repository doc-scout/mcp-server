// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type OpenNodesArgs struct {
	Names []string `json:"names" jsonschema:"names of nodes to open"`
}

func openNodesHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args OpenNodesArgs) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args OpenNodesArgs) (*mcp.CallToolResult, memory.KnowledgeGraph, error) {
		g, err := graph.OpenNodes(args.Names)
		if err != nil {
			return nil, memory.KnowledgeGraph{}, err
		}
		return nil, g, nil
	}
}
