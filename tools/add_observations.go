// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AddObservationsArgs struct {
	Observations []memory.Observation `json:"observations" jsonschema:"observations to add"`
}

type AddObservationsResult struct {
	Observations []memory.Observation `json:"observations"`
}

func addObservationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {
		observations, err := graph.AddObservations(args.Observations)
		if err != nil {
			return nil, AddObservationsResult{}, err
		}
		return nil, AddObservationsResult{Observations: observations}, nil
	}
}
