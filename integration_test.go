// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/leonancarvalho/docscout-mcp/scanner"
	"github.com/leonancarvalho/docscout-mcp/tools"
)

// mockScanner provides a minimal scanner for E2E tests.
type mockScanner struct{}

func (m *mockScanner) ListRepos() []scanner.RepoInfo {
	return []scanner.RepoInfo{
		{
			Name:        "test-repo",
			FullName:    "test-org/test-repo",
			Description: "A test repository",
			HTMLURL:     "https://github.com/test-org/test-repo",
			Files: []scanner.FileEntry{
				{RepoName: "test-repo", Path: "README.md", Type: "readme"},
				{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
			},
		},
	}
}

func (m *mockScanner) SearchDocs(query string) []scanner.FileEntry {
	if query == "guide" {
		return []scanner.FileEntry{
			{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
		}
	}
	return nil
}

func (m *mockScanner) GetFileContent(ctx context.Context, repo, path string) (string, error) {
	if repo == "test-repo" && path == "README.md" {
		return "# Test Repo\nThis is a test.", nil
	}
	return "", nil
}

// setupTestServer creates a full MCP server with all tools registered,
// connects a client via in-memory transport, and returns the client session.
func setupTestServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "docscout-mcp-test",
		Version: "test",
	}, nil)

	// Register scanner tools
	tools.Register(server, &mockScanner{})

	// Register memory tools (in-memory SQLite)
	memory.Register(server, "")

	// Wire up in-memory transport (no real stdio needed)
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	return session
}

func TestE2E_ListTools(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// We expect 3 scanner tools + 9 memory tools = 12 total
	if len(result.Tools) < 12 {
		t.Fatalf("expected at least 12 tools, got %d", len(result.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	expected := []string{
		"list_repos", "search_docs", "get_file_content",
		"create_entities", "create_relations", "add_observations",
		"delete_entities", "delete_observations", "delete_relations",
		"read_graph", "search_nodes", "open_nodes",
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestE2E_ListRepos(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "list_repos",
	})
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

func TestE2E_SearchDocs(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
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

func TestE2E_GetFileContent(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_file_content",
		Arguments: map[string]any{"repo": "test-repo", "path": "README.md"},
	})
	if err != nil {
		t.Fatalf("CallTool get_file_content: %v", err)
	}

	if result.IsError {
		t.Fatalf("get_file_content returned error")
	}

	text := result.Content[0].(*mcp.TextContent).Text
	if text == "" {
		t.Fatal("expected non-empty file content")
	}
}

func TestE2E_MemoryLifecycle(t *testing.T) {
	session := setupTestServer(t)
	defer session.Close()

	ctx := context.Background()

	// 1. Create entities (observations as string arrays)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_entities",
		Arguments: map[string]any{
			"entities": []map[string]any{
				{"name": "api-gateway", "entityType": "Component", "observations": []string{"routes traffic"}},
				{"name": "user-service", "entityType": "Component", "observations": []string{}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create_entities: %v", err)
	}
	if res.IsError {
		t.Fatalf("create_entities returned error: %v", res.Content)
	}

	// 2. Create relation
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_relations",
		Arguments: map[string]any{
			"relations": []map[string]any{
				{"from": "api-gateway", "to": "user-service", "relationType": "proxies"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create_relations: %v", err)
	}
	if res.IsError {
		t.Fatalf("create_relations returned error: %v", res.Content)
	}

	// 3. Search nodes
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_nodes",
		Arguments: map[string]any{"query": "gateway"},
	})
	if err != nil {
		t.Fatalf("search_nodes: %v", err)
	}
	if res.IsError {
		t.Fatalf("search_nodes returned error: %v", res.Content)
	}

	// 4. Read full graph
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "read_graph",
	})
	if err != nil {
		t.Fatalf("read_graph: %v", err)
	}
	if res.IsError {
		t.Fatalf("read_graph returned error: %v", res.Content)
	}

	// 5. Delete entity (should cascade relations)
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "delete_entities",
		Arguments: map[string]any{
			"entityNames": []string{"api-gateway"},
		},
	})
	if err != nil {
		t.Fatalf("delete_entities: %v", err)
	}
	if res.IsError {
		t.Fatalf("delete_entities returned error: %v", res.Content)
	}
}
