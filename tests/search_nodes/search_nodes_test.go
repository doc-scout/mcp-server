// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package search_nodes_test

import (
	"context"
	"testing"

	"github.com/leonancarvalho/docscout-mcp/tests/testutils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2E_SearchNodes(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(session.Close)

	ctx := t.Context()

	// Add data
	_, _ = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_entities",
		Arguments: map[string]any{
			"entities": []map[string]any{
				{"name": "auth-service", "entityType": "Service", "observations": []string{"jwt"}},
			},
		},
	})

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_nodes",
		Arguments: map[string]any{"query": "auth"},
	})
	if err != nil {
		t.Fatalf("search_nodes: %v", err)
	}
	if res.IsError {
		t.Fatalf("search_nodes returned error: %v", res.Content)
	}
}
