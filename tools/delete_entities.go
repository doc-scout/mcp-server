// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteEntitiesArgs struct {
	EntityNames []string `json:"entityNames" jsonschema:"entities to delete"`
}

func deleteEntitiesHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args DeleteEntitiesArgs) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args DeleteEntitiesArgs) (*mcp.CallToolResult, any, error) {
		err := graph.DeleteEntities(args.EntityNames)
		return nil, nil, err
	}
}
