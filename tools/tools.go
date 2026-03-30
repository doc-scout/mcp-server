// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"log"
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
				log.Printf("[tools] MCP tool panicked: tool=%s panic=%v\nstack=%s", name, r, string(debug.Stack()))
				err = fmt.Errorf("internal server error in tool '%s' (panic recovered: %v)", name, r)
			}
		}()
		return handler(ctx, req, args)
	}
}

// Register adds all DocScout MCP tools to the server.
// graph and search may be nil — get_scan_status degrades gracefully, search_content is omitted.
func Register(s *mcp.Server, sc DocumentScanner, graph GraphStore, search ContentSearcher) {
	// --- Scanner Tools ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_repos",
		Description: "Lists all repositories in the organization that contain documentation files (catalog-info.yaml, mkdocs.yml, openapi.yaml, swagger.json, README.md, docs/*.md).",
	}, withRecovery("list_repos", listReposHandler(sc)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_docs",
		Description: "Searches for documentation files by matching a query term against file paths and repository names.",
	}, withRecovery("search_docs", searchDocsHandler(sc)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_content",
		Description: "Retrieves the raw content of a specific documentation file from a GitHub repository. Note: For security reasons, this tool will only return files that have been successfully indexed as documentation (i.e. returned by list_repos or search_docs).",
	}, withRecovery("get_file_content", getFileContentHandler(sc)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_scan_status",
		Description: "Returns the current state of the documentation scanner and knowledge graph index. Call this before searching to confirm the index is populated, especially right after startup.",
	}, withRecovery("get_scan_status", getScanStatusHandler(sc, graph, search)))

	if search != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "search_content",
			Description: "Full-text search across the content of all cached documentation files. Use this to find which service handles a specific responsibility (e.g. 'payment', 'authentication'). Only available when SCAN_CONTENT=true.",
		}, withRecovery("search_content", searchContentHandler(search)))
	}

	// --- Memory / Knowledge Graph Tools ---
	if graph != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "create_entities",
			Description: "Create multiple new entities in the knowledge graph",
		}, withRecovery("create_entities", createEntitiesHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "create_relations",
			Description: "Create multiple new relations between entities",
		}, withRecovery("create_relations", createRelationsHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "add_observations",
			Description: "Add new observations to existing entities",
		}, withRecovery("add_observations", addObservationsHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "delete_entities",
			Description: "Remove entities and their relations",
		}, withRecovery("delete_entities", deleteEntitiesHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "delete_observations",
			Description: "Remove specific observations from entities",
		}, withRecovery("delete_observations", deleteObservationsHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "delete_relations",
			Description: "Remove specific relations from the graph",
		}, withRecovery("delete_relations", deleteRelationsHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "read_graph",
			Description: "Read the entire knowledge graph",
		}, withRecovery("read_graph", readGraphHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "search_nodes",
			Description: "Search for nodes based on query",
		}, withRecovery("search_nodes", searchNodesHandler(graph)))

		mcp.AddTool(s, &mcp.Tool{
			Name:        "open_nodes",
			Description: "Retrieve specific nodes by name",
		}, withRecovery("open_nodes", openNodesHandler(graph)))
	}
}
