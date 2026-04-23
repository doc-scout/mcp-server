// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

const auditDisabledMsg = "audit persistence not enabled — set DATABASE_URL to a persistent store"

// QueryAuditLogArgs are the input parameters for query_audit_log.
type QueryAuditLogArgs struct {
	Agent     string `json:"agent,omitempty"     jsonschema:"Filter events by agent name (e.g. claude-desktop, indexer-bot)."`
	Tool      string `json:"tool,omitempty"      jsonschema:"Filter events by MCP tool name (e.g. create_entities, delete_entities)."`
	Operation string `json:"operation,omitempty" jsonschema:"Filter by operation type: create, delete, update, or add."`
	Outcome   string `json:"outcome,omitempty"   jsonschema:"Filter by outcome: ok or error."`
	Since     string `json:"since,omitempty"     jsonschema:"Return only events after this RFC3339 timestamp (e.g. 2026-04-21T00:00:00Z)."`
	Limit     int    `json:"limit,omitempty"     jsonschema:"Maximum events to return (default 50, max 500)."`
}

// QueryAuditLogResult is the structured output of query_audit_log.
type QueryAuditLogResult struct {
	Events []memory.AuditEvent `json:"events"`
	Total  int64               `json:"total"`
}

func queryAuditLogHandler(r AuditReader) func(ctx context.Context, req *mcp.CallToolRequest, args QueryAuditLogArgs) (*mcp.CallToolResult, QueryAuditLogResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args QueryAuditLogArgs) (*mcp.CallToolResult, QueryAuditLogResult, error) {
		if r == nil {
			return nil, QueryAuditLogResult{}, fmt.Errorf(auditDisabledMsg)
		}

		filter := memory.AuditFilter{
			Agent:     args.Agent,
			Tool:      args.Tool,
			Operation: args.Operation,
			Outcome:   args.Outcome,
			Limit:     args.Limit,
		}
		if args.Since != "" {
			t, err := time.Parse(time.RFC3339, args.Since)
			if err != nil {
				return nil, QueryAuditLogResult{}, fmt.Errorf("invalid 'since' timestamp: %w", err)
			}
			filter.Since = t
		}

		events, total, err := r.Query(ctx, filter)
		if err != nil {
			return nil, QueryAuditLogResult{}, fmt.Errorf("query_audit_log: %w", err)
		}
		if events == nil {
			events = []memory.AuditEvent{}
		}
		return nil, QueryAuditLogResult{Events: events, Total: total}, nil
	}
}
