// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

type CreateEntitiesArgs struct {
	Entities []coregraph.Entity `json:"entities" jsonschema:"entities to create"`
}

type CreateEntitiesResult struct {
	Entities []coregraph.Entity `json:"entities"`

	// Skipped lists observations rejected by quality guards (empty, too short/long, duplicate).

	// Non-nil only when at least one observation was filtered.

	Skipped []SkippedObservation `json:"skipped,omitempty"`
}

func createEntitiesHandler(graph GraphStore, semantic SemanticSearch) func(ctx context.Context, req *mcp.CallToolRequest, args CreateEntitiesArgs) (*mcp.CallToolResult, CreateEntitiesResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args CreateEntitiesArgs) (*mcp.CallToolResult, CreateEntitiesResult, error) {

		var clean []coregraph.Entity

		var allSkipped []SkippedObservation

		for _, e := range args.Entities {

			valid, skipped := sanitizeObservations(e.Name, e.Observations)

			allSkipped = append(allSkipped, skipped...)

			clean = append(clean, coregraph.Entity{

				Name: e.Name,

				EntityType: e.EntityType,

				Observations: valid,
			})

		}

		entities, err := graph.CreateEntities(clean)

		if err != nil {

			return nil, CreateEntitiesResult{}, err

		}

		if semantic != nil {

			names := make([]string, len(entities))

			for i, e := range entities {

				names[i] = e.Name

			}

			semantic.ScheduleIndexEntities(names)

		}

		return nil, CreateEntitiesResult{Entities: entities, Skipped: allSkipped}, nil

	}

}


