// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TriggerScanArgs struct{}

type TriggerScanResult struct {

	// Triggered is true when a scan was newly queued, false when one was already pending.

	Triggered bool `json:"triggered"`

	// AlreadyQueued is true when a scan was already pending and this request was a no-op.

	AlreadyQueued bool `json:"already_queued"`
}

func triggerScanHandler(sc DocumentScanner) func(ctx context.Context, req *mcp.CallToolRequest, args TriggerScanArgs) (*mcp.CallToolResult, TriggerScanResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args TriggerScanArgs) (*mcp.CallToolResult, TriggerScanResult, error) {

		queued := sc.TriggerScan()

		if queued {

			slog.Info("[trigger_scan] On-demand full scan queued by MCP tool call")

		} else {

			slog.Info("[trigger_scan] On-demand scan already queued — request coalesced")

		}

		return nil, TriggerScanResult{

			Triggered: queued,

			AlreadyQueued: !queued,
		}, nil

	}

}
