// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp_discovery_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/tests/testutils"
	"github.com/doc-scout/mcp-server/tools"
)

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s returned MCP error: %v", name, res.Content)
	}
	if len(res.Content) == 0 {
		return "{}"
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}
	return text.Text
}

func seedMCPServer(t *testing.T, session *mcp.ClientSession, name, repo, transport string, toolObs []string) {
	t.Helper()
	obs := []string{
		"transport:" + transport,
		"_scan_repo:" + repo,
		"config_file:.mcp.json",
	}
	obs = append(obs, toolObs...)

	callTool(t, session, "create_entities", map[string]any{
		"entities": []map[string]any{
			{"name": name, "entityType": "mcp-server", "observations": obs},
		},
	})
}

func TestDiscoverMCPServers_Inventory(t *testing.T) {
	session := testutils.SetupTestServer(t)
	seedMCPServer(t, session, "github", "test-org/test-repo", "stdio",
		[]string{"tool:search_repositories: Search GitHub repos"})

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v — raw: %s", err, raw)
	}
	if result.Total == 0 {
		t.Fatal("expected at least one mcp-server")
	}
	if result.Servers[0].Name != "github" {
		t.Fatalf("want name=github, got %q", result.Servers[0].Name)
	}
}

func TestDiscoverMCPServers_CapabilitySearch(t *testing.T) {
	session := testutils.SetupTestServer(t)
	seedMCPServer(t, session, "github", "org/repo-a", "stdio",
		[]string{"tool:search_repositories: Search GitHub repos"})
	seedMCPServer(t, session, "postgres", "org/repo-b", "stdio",
		[]string{"tool:query: Execute a SQL query"})

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{
		"tool_name": "search",
	})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("want total=1 (github only), got %d", result.Total)
	}
	if result.Servers[0].Name != "github" {
		t.Fatalf("want github, got %q", result.Servers[0].Name)
	}
}

func TestDiscoverMCPServers_RepoFilter(t *testing.T) {
	session := testutils.SetupTestServer(t)
	seedMCPServer(t, session, "github", "org/repo-a", "stdio", nil)
	seedMCPServer(t, session, "fetch", "org/repo-b", "http", nil)

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{
		"repo": "org/repo-a",
	})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 1 || result.Servers[0].Name != "github" {
		t.Fatalf("want only github from repo-a, got %+v", result)
	}
}

func TestDiscoverMCPServers_TraverseUsesMCPEdge(t *testing.T) {
	session := testutils.SetupTestServer(t)

	callTool(t, session, "create_entities", map[string]any{
		"entities": []map[string]any{
			{"name": "my-service", "entityType": "service", "observations": []string{"uses github mcp"}},
			{"name": "github", "entityType": "mcp-server", "observations": []string{"transport:stdio", "_scan_repo:org/my-repo"}},
		},
	})
	callTool(t, session, "create_relations", map[string]any{
		"relations": []map[string]any{
			{"from": "my-service", "to": "github", "relationType": "uses_mcp"},
		},
	})

	raw := callTool(t, session, "traverse_graph", map[string]any{
		"entity":        "my-service",
		"relation_type": "uses_mcp",
		"direction":     "outgoing",
		"depth":         1,
	})

	var result tools.TraverseGraphResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal TraverseGraphResult: %v — raw: %s", err, raw)
	}
	if result.TotalFound != 1 {
		t.Fatalf("want 1 node (github), got %d", result.TotalFound)
	}
	if result.Nodes[0].Name != "github" {
		t.Fatalf("want github, got %q", result.Nodes[0].Name)
	}
}

func TestDiscoverMCPServers_EmptyResult(t *testing.T) {
	session := testutils.SetupTestServer(t)

	raw := callTool(t, session, "discover_mcp_servers", map[string]any{
		"tool_name": "nonexistent-tool-xyz",
	})
	var result tools.DiscoverMCPServersResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Total != 0 {
		t.Fatalf("want total=0, got %d", result.Total)
	}
	if result.Servers == nil {
		t.Fatal("Servers must be non-nil empty slice (not null in JSON)")
	}
}
