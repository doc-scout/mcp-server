// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// massDeleteThreshold is the maximum number of entities that can be deleted in a single

// call without setting confirm=true. This guards against accidental graph wipes by AI agents.

const massDeleteThreshold = 10

type DeleteEntitiesArgs struct {
	EntityNames []string `json:"entityNames" jsonschema:"Entities to delete."`

	Confirm bool `json:"confirm,omitempty" jsonschema:"Must be set to true when deleting more than 10 entities at once. This is a safety guard to prevent accidental mass deletions."`
}

func deleteEntitiesHandler(graph GraphStore, semantic SemanticSearch) func(ctx context.Context, req *mcp.CallToolRequest, args DeleteEntitiesArgs) (*mcp.CallToolResult, any, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args DeleteEntitiesArgs) (*mcp.CallToolResult, any, error) {

		if len(args.EntityNames) > massDeleteThreshold && !args.Confirm {

			return nil, nil, fmt.Errorf(

				"safety guard: refusing to delete %d entities in a single call (threshold: %d) — "+

					"set confirm=true to proceed; double-check that you are not deleting the entire graph by mistake",

				len(args.EntityNames), massDeleteThreshold,
			)

		}

		err := graph.DeleteEntities(args.EntityNames)

		if err == nil && semantic != nil {

			semantic.ScheduleIndexEntities(args.EntityNames)

		}

		return nil, nil, err

	}

}
