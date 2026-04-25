// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type UpdateEntityArgs struct {
	Name string `json:"name"               jsonschema:"Current name of the entity to update. Must match exactly."`

	NewName string `json:"new_name,omitempty" jsonschema:"New name for the entity. When provided, all relations and observations referencing this entity are updated atomically."`

	NewType string `json:"new_type,omitempty" jsonschema:"New entity type (e.g. 'service', 'team', 'api'). Omit to keep the current type."`
}

type UpdateEntityResult struct {
	Updated bool `json:"updated"`

	Name string `json:"name"` // effective name after update

}

func updateEntityHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args UpdateEntityArgs) (*mcp.CallToolResult, UpdateEntityResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args UpdateEntityArgs) (*mcp.CallToolResult, UpdateEntityResult, error) {

		if strings.TrimSpace(args.Name) == "" {

			return nil, UpdateEntityResult{}, fmt.Errorf("parameter 'name' is required")

		}

		if strings.TrimSpace(args.NewName) == "" && strings.TrimSpace(args.NewType) == "" {

			return nil, UpdateEntityResult{}, fmt.Errorf("at least one of 'new_name' or 'new_type' must be provided")

		}

		if err := graph.UpdateEntity(args.Name, args.NewName, args.NewType); err != nil {

			return nil, UpdateEntityResult{}, fmt.Errorf("update_entity: %w", err)

		}

		effectiveName := args.Name

		if args.NewName != "" {

			effectiveName = args.NewName

		}

		return nil, UpdateEntityResult{Updated: true, Name: effectiveName}, nil

	}

}

