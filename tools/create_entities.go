// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateEntitiesArgs struct {
	Entities []memory.Entity `json:"entities" jsonschema:"entities to create"`
}

type CreateEntitiesResult struct {
	Entities []memory.Entity `json:"entities"`
	// Skipped lists observations rejected by quality guards (empty, too short/long, duplicate).
	// Non-nil only when at least one observation was filtered.
	Skipped []SkippedObservation `json:"skipped,omitempty"`
}

func createEntitiesHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args CreateEntitiesArgs) (*mcp.CallToolResult, CreateEntitiesResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args CreateEntitiesArgs) (*mcp.CallToolResult, CreateEntitiesResult, error) {
		var clean []memory.Entity
		var allSkipped []SkippedObservation

		for _, e := range args.Entities {
			valid, skipped := sanitizeObservations(e.Name, e.Observations)
			allSkipped = append(allSkipped, skipped...)
			clean = append(clean, memory.Entity{
				Name:         e.Name,
				EntityType:   e.EntityType,
				Observations: valid,
			})
		}

		entities, err := graph.CreateEntities(clean)
		if err != nil {
			return nil, CreateEntitiesResult{}, err
		}
		return nil, CreateEntitiesResult{Entities: entities, Skipped: allSkipped}, nil
	}
}
