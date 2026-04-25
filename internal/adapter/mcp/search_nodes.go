// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

type SearchNodesArgs struct {
	Query string `json:"query" jsonschema:"query string"`

	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"When true, includes entities marked as archived (_status:archived). Default false — archived entities are hidden from results."`
}

func searchNodesHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args SearchNodesArgs) (*mcp.CallToolResult, coregraph.KnowledgeGraph, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchNodesArgs) (*mcp.CallToolResult, coregraph.KnowledgeGraph, error) {

		g, err := graph.SearchNodesFiltered(args.Query, args.IncludeArchived)

		if err != nil {

			return nil, coregraph.KnowledgeGraph{}, err

		}

		return nil, g, nil

	}

}
