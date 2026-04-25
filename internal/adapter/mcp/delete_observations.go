// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

type DeleteObservationsArgs struct {
	Deletions []coregraph.Observation `json:"deletions" jsonschema:"observations to delete"`
}

func deleteObservationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args DeleteObservationsArgs) (*mcp.CallToolResult, any, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args DeleteObservationsArgs) (*mcp.CallToolResult, any, error) {

		err := graph.DeleteObservations(args.Deletions)

		return nil, nil, err

	}

}


