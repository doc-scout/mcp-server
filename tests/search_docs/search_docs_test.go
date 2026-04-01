// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package search_docs_test

import (
	"testing"

	"github.com/leonancarvalho/docscout-mcp/tests/testutils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2E_SearchDocs(t *testing.T) {
	session := testutils.SetupTestServer(t)
	t.Cleanup(session.Close)

	ctx := t.Context()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_docs",
		Arguments: map[string]any{"query": "guide"},
	})
	if err != nil {
		t.Fatalf("CallTool search_docs: %v", err)
	}
	if result.IsError {
		t.Fatalf("search_docs returned error")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content from search_docs")
	}
}
