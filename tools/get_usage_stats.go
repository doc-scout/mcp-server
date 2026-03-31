// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UsageStatsArgs has no parameters — the tool returns all stats unconditionally.
type UsageStatsArgs struct{}

// UsageStatsResult contains per-tool call counts since server start.
type UsageStatsResult struct {
	ToolCalls map[string]int64 `json:"tool_calls" jsonschema:"A map of tool name to the total number of times it has been called since the server started."`
}

func getUsageStatsHandler(m *ToolMetrics) func(ctx context.Context, req *mcp.CallToolRequest, args UsageStatsArgs) (*mcp.CallToolResult, UsageStatsResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args UsageStatsArgs) (*mcp.CallToolResult, UsageStatsResult, error) {
		return nil, UsageStatsResult{ToolCalls: m.Snapshot()}, nil
	}
}
