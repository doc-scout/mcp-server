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
	// Skipped lists observations rejected by quality guards (empty, too short/long, duplicate).
	// Non-nil only when at least one observation was filtered.
	Skipped []SkippedObservation `json:"skipped,omitempty"`
}

func addObservationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {
		var clean []memory.Observation
		var allSkipped []SkippedObservation

		for _, obs := range args.Observations {
			valid, skipped := sanitizeObservations(obs.EntityName, obs.Contents)
			allSkipped = append(allSkipped, skipped...)
			if len(valid) > 0 {
				clean = append(clean, memory.Observation{
					EntityName: obs.EntityName,
					Contents:   valid,
				})
			}
		}

		if len(clean) == 0 {
			return nil, AddObservationsResult{Skipped: allSkipped}, nil
		}

		observations, err := graph.AddObservations(clean)
		if err != nil {
			return nil, AddObservationsResult{}, err
		}
		return nil, AddObservationsResult{Observations: observations, Skipped: allSkipped}, nil
	}
}
