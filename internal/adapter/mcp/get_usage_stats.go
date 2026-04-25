// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UsageStatsArgs has no parameters — the tool returns all stats unconditionally.

type UsageStatsArgs struct{}

// UsageStatsResult contains per-tool call counts and top-accessed documents since server start.

type UsageStatsResult struct {
	ToolCalls map[string]int64 `json:"tool_calls"    jsonschema:"A map of tool name to the total number of times it has been called since the server started."`

	TopDocuments []DocAccess `json:"top_documents" jsonschema:"The 20 most-fetched documents since server start, sorted by access count descending. Useful for identifying which services or files are most consulted by AI agents."`
}

const topDocumentsN = 20

func getUsageStatsHandler(m *ToolMetrics, d *DocMetrics) func(ctx context.Context, req *mcp.CallToolRequest, args UsageStatsArgs) (*mcp.CallToolResult, UsageStatsResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args UsageStatsArgs) (*mcp.CallToolResult, UsageStatsResult, error) {

		return nil, UsageStatsResult{

			ToolCalls: m.Snapshot(),

			TopDocuments: d.TopN(topDocumentsN),
		}, nil

	}

}

