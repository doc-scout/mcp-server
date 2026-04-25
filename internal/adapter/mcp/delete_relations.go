// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

type DeleteRelationsArgs struct {
	Relations []coregraph.Relation `json:"relations" jsonschema:"relations to delete"`
}

func deleteRelationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args DeleteRelationsArgs) (*mcp.CallToolResult, struct{}, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args DeleteRelationsArgs) (*mcp.CallToolResult, struct{}, error) {

		err := graph.DeleteRelations(args.Relations)

		return nil, struct{}{}, err

	}

}


