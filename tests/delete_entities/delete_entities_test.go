// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package delete_entities_test

import (
	"context"
	"testing"

	"github.com/leonancarvalho/docscout-mcp/tests/testutils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2E_DeleteEntities(t *testing.T) {
	session := testutils.SetupTestServer(t)
	defer session.Close()

	ctx := context.Background()

	// Setup: create entity
	_, _ = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_entities",
		Arguments: map[string]any{
			"entities": []map[string]any{
				{"name": "obsolete-service", "entityType": "Service", "observations": []string{}},
			},
		},
	})
	
	// Test deleting
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "delete_entities",
		Arguments: map[string]any{"entityNames": []string{"obsolete-service"}},
	})
	if err != nil {
		t.Fatalf("delete_entities: %v", err)
	}
	if res.IsError {
		t.Fatalf("delete_entities returned error: %v", res.Content)
	}
}
