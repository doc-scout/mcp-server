// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// withRecovery wraps an MCP tool handler to catch and log panics gracefully.
func withRecovery[A, R any](
	name string,
	handler func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error),
) func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args A) (res *mcp.CallToolResult, ret R, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("[tools] MCP tool panicked", "tool", name, "panic", r, "stack", string(debug.Stack()))
				err = fmt.Errorf("internal server error in tool '%s' (panic recovered: %v)", name, r)
			}
		}()
		return handler(ctx, req, args)
	}
}

// withMetrics wraps a handler to record a call in ToolMetrics before execution.
func withMetrics[A, R any](
	name string,
	m *ToolMetrics,
	handler func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error),
) func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error) {
		m.Record(name)
		return handler(ctx, req, args)
	}
}

// Register adds all DocScout MCP tools to the server.
// graph and search may be nil — get_scan_status degrades gracefully, search_content is omitted.
// metrics and docMetrics must not be nil.
func Register(s *mcp.Server, sc DocumentScanner, graph GraphStore, search ContentSearcher, metrics *ToolMetrics, docMetrics *DocMetrics) {
	// --- Scanner Tools ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_repos",
		Description: "Lists all repositories in the organization that contain documentation files (catalog-info.yaml, mkdocs.yml, openapi.yaml, swagger.json, README.md, docs/*.md).",
	}, withMetrics("list_repos", metrics, withRecovery("list_repos", listReposHandler(sc))))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_docs",
		Description: "Searches for documentation files by matching a query term against file paths and repository names.",
	}, withMetrics("search_docs", metrics, withRecovery("search_docs", searchDocsHandler(sc))))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_content",
		Description: "Retrieves the raw content of a specific documentation file from a GitHub repository. Note: For security reasons, this tool will only return files that have been successfully indexed as documentation (i.e. returned by list_repos or search_docs).",
	}, withMetrics("get_file_content", metrics, withRecovery("get_file_content", getFileContentHandler(sc, docMetrics))))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_scan_status",
		Description: "Returns the current state of the documentation scanner and knowledge graph index. Call this before searching to confirm the index is populated, especially right after startup.",
	}, withMetrics("get_scan_status", metrics, withRecovery("get_scan_status", getScanStatusHandler(sc, graph, search))))

	if search != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "search_content",
			Description: "Full-text search across the content of all cached documentation files. Use this to find which service handles a specific responsibility (e.g. 'payment', 'authentication'). Only available when SCAN_CONTENT=true.",
		}, withMetrics("search_content", metrics, withRecovery("search_content", searchContentHandler(search, docMetrics))))
	}

	// --- Memory / Knowledge Graph Tools ---
	if graph != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "create_entities",
			Description: "Create multiple new entities in the knowledge graph",
		}, withMetrics("create_entities", metrics, withRecovery("create_entities", createEntitiesHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "create_relations",
			Description: "Create multiple new relations between entities",
		}, withMetrics("create_relations", metrics, withRecovery("create_relations", createRelationsHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "add_observations",
			Description: "Add new observations to existing entities",
		}, withMetrics("add_observations", metrics, withRecovery("add_observations", addObservationsHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "delete_entities",
			Description: fmt.Sprintf("Remove entities and their associated relations from the knowledge graph. Deleting more than %d entities in a single call requires confirm=true as a safety guard against accidental graph wipes.", massDeleteThreshold),
		}, withMetrics("delete_entities", metrics, withRecovery("delete_entities", deleteEntitiesHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "delete_observations",
			Description: "Remove specific observations from entities",
		}, withMetrics("delete_observations", metrics, withRecovery("delete_observations", deleteObservationsHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "delete_relations",
			Description: "Remove specific relations from the graph",
		}, withMetrics("delete_relations", metrics, withRecovery("delete_relations", deleteRelationsHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "read_graph",
			Description: "Read the entire knowledge graph",
		}, withMetrics("read_graph", metrics, withRecovery("read_graph", readGraphHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "search_nodes",
			Description: "Search for nodes based on query",
		}, withMetrics("search_nodes", metrics, withRecovery("search_nodes", searchNodesHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "open_nodes",
			Description: "Retrieve specific nodes by name",
		}, withMetrics("open_nodes", metrics, withRecovery("open_nodes", openNodesHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{
			Name: "traverse_graph",
			Description: `Traverses the knowledge graph from a starting entity, following directed edges up to a given depth.
Use this instead of read_graph when you need to answer focused questions about a specific service — it returns only the relevant subgraph without loading every entity.
Examples:
  direction=outgoing, relation_type=depends_on  → transitive dependency tree of a service
  direction=incoming, relation_type=consumes_api → all services that consume a given API
  direction=both, depth=2                        → full two-hop neighbourhood of a service`,
		}, withMetrics("traverse_graph", metrics, withRecovery("traverse_graph", traverseGraphHandler(graph))))
	}

	// --- Observability ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_usage_stats",
		Description: "Returns how many times each MCP tool has been called and the top 20 most-fetched documents since server start. Use this to identify which documentation areas are most frequently accessed by AI agents, helping teams spot knowledge gaps.",
	}, withRecovery("get_usage_stats", getUsageStatsHandler(metrics, docMetrics)))
}
