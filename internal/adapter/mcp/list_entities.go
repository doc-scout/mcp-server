// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

type ListEntitiesArgs struct {
	EntityType string `json:"entity_type,omitempty" jsonschema:"optional filter: only return entities of this type (e.g. 'service', 'team', 'event-topic', 'grpc-service', 'api', 'person'). Leave empty to return all entities."`
}

func listEntitiesHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args ListEntitiesArgs) (*mcp.CallToolResult, coregraph.KnowledgeGraph, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args ListEntitiesArgs) (*mcp.CallToolResult, coregraph.KnowledgeGraph, error) {

		g, err := graph.ListEntities(args.EntityType)

		if err != nil {

			return nil, coregraph.KnowledgeGraph{}, err

		}

		return nil, g, nil

	}

}
