// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

func readGraphHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, coregraph.KnowledgeGraph, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, coregraph.KnowledgeGraph, error) {

		g, err := graph.ReadGraph()

		if err != nil {

			return nil, coregraph.KnowledgeGraph{}, err

		}

		return nil, g, nil

	}

}
