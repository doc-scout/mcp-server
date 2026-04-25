// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

// DiscoverMCPServersArgs are the input parameters for discover_mcp_servers.

type DiscoverMCPServersArgs struct {
	Repo string `json:"repo,omitzero"      jsonschema:"Filter results to a specific repository name."`

	ToolName string `json:"tool_name,omitzero" jsonschema:"Return only MCP servers that have a tool matching this name (capability search). Matched case-insensitively against tool observation prefixes."`

	Transport string `json:"transport,omitzero" jsonschema:"Filter by transport type: stdio, http, or sse."`

	Limit int `json:"limit,omitzero"     jsonschema:"Maximum number of servers to return (default 20, max 100)."`
}

// MCPServerResult is one discovered MCP server.

type MCPServerResult struct {
	Name string `json:"name"`

	Repo string `json:"repo"`

	Transport string `json:"transport,omitzero"`

	Command string `json:"command,omitzero"`

	URL string `json:"url,omitzero"`

	Tools []string `json:"tools"`

	ConfigFile string `json:"config_file,omitzero"`
}

// DiscoverMCPServersResult is the structured output of discover_mcp_servers.

type DiscoverMCPServersResult struct {
	Servers []MCPServerResult `json:"servers"`

	Total int `json:"total"`
}

func discoverMCPServersHandler(g GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args DiscoverMCPServersArgs) (*mcp.CallToolResult, DiscoverMCPServersResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args DiscoverMCPServersArgs) (*mcp.CallToolResult, DiscoverMCPServersResult, error) {

		limit := args.Limit

		if limit <= 0 {

			limit = 20

		}

		if limit > 100 {

			limit = 100

		}

		kg, err := g.ListEntities("mcp-server")

		if err != nil {

			return nil, DiscoverMCPServersResult{}, fmt.Errorf("discover_mcp_servers: %w", err)

		}

		var servers []MCPServerResult

		for _, entity := range kg.Entities {

			srv := extractMCPServer(entity)

			if args.Repo != "" && srv.Repo != args.Repo {

				continue

			}

			if args.Transport != "" && srv.Transport != args.Transport {

				continue

			}

			if args.ToolName != "" && !hasMatchingTool(srv.Tools, args.ToolName) {

				continue

			}

			servers = append(servers, srv)

			if len(servers) >= limit {

				break

			}

		}

		if servers == nil {

			servers = []MCPServerResult{}

		}

		return nil, DiscoverMCPServersResult{Servers: servers, Total: len(servers)}, nil

	}

}

// extractMCPServer converts a graph entity into an MCPServerResult by parsing observations.

func extractMCPServer(entity memory.Entity) MCPServerResult {

	srv := MCPServerResult{Name: entity.Name}

	for _, obs := range entity.Observations {

		switch {

		case strings.HasPrefix(obs, "transport:"):

			srv.Transport = strings.TrimPrefix(obs, "transport:")

		case strings.HasPrefix(obs, "command:"):

			srv.Command = strings.TrimPrefix(obs, "command:")

		case strings.HasPrefix(obs, "url:"):

			srv.URL = strings.TrimPrefix(obs, "url:")

		case strings.HasPrefix(obs, "config_file:"):

			srv.ConfigFile = strings.TrimPrefix(obs, "config_file:")

		case strings.HasPrefix(obs, "_scan_repo:"):

			srv.Repo = strings.TrimPrefix(obs, "_scan_repo:")

		case strings.HasPrefix(obs, "tool:"):

			rest := strings.TrimPrefix(obs, "tool:")

			toolName, _, _ := strings.Cut(rest, ":")

			if toolName != "" {

				srv.Tools = append(srv.Tools, strings.TrimSpace(toolName))

			}

		}

	}

	if srv.Tools == nil {

		srv.Tools = []string{}

	}

	return srv

}

// hasMatchingTool returns true if any tool name contains the query (case-insensitive).

func hasMatchingTool(tools []string, query string) bool {

	q := strings.ToLower(query)

	for _, t := range tools {

		if strings.Contains(strings.ToLower(t), q) {

			return true

		}

	}

	return false

}
