// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

type CreateRelationsArgs struct {
	Relations []coregraph.Relation `json:"relations" jsonschema:"relations to create"`
}

type CreateRelationsResult struct {
	Relations []coregraph.Relation `json:"relations"`
}

func createRelationsHandler(graph GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args CreateRelationsArgs) (*mcp.CallToolResult, CreateRelationsResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args CreateRelationsArgs) (*mcp.CallToolResult, CreateRelationsResult, error) {

		for i := range args.Relations {

			if args.Relations[i].Confidence == "" {

				args.Relations[i].Confidence = "authoritative"

			}

		}

		relations, err := graph.CreateRelations(args.Relations)

		if err != nil {

			return nil, CreateRelationsResult{}, err

		}

		return nil, CreateRelationsResult{Relations: relations}, nil

	}

}


