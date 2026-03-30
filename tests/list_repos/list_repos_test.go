// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package list_repos_test

import (
	"context"
	"testing"

	"github.com/leonancarvalho/docscout-mcp/tests/testutils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2E_ListRepos(t *testing.T) {
	session := testutils.SetupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_repos"})
	if err != nil {
		t.Fatalf("CallTool list_repos: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_repos returned error")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content from list_repos")
	}
}
