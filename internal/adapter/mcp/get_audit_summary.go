// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
)

// GetAuditSummaryArgs are the input parameters for get_audit_summary.

type GetAuditSummaryArgs struct {
	WindowHours int `json:"window_hours,omitzero" jsonschema:"Time window in hours to summarise (default 24, max 720). Events older than this are excluded."`
}

// GetAuditSummaryResult is the structured output of get_audit_summary.

type GetAuditSummaryResult struct {
	WindowHours int `json:"window_hours"`

	Summary coreaudit.AuditSummary `json:"summary"`
}

func getAuditSummaryHandler(r AuditReader) func(ctx context.Context, req *mcp.CallToolRequest, args GetAuditSummaryArgs) (*mcp.CallToolResult, GetAuditSummaryResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args GetAuditSummaryArgs) (*mcp.CallToolResult, GetAuditSummaryResult, error) {

		if r == nil {

			return nil, GetAuditSummaryResult{}, errors.New(auditDisabledMsg)

		}

		hours := args.WindowHours

		if hours <= 0 {

			hours = 24

		}

		if hours > 720 {

			hours = 720

		}

		summary, err := r.Summary(ctx, time.Duration(hours)*time.Hour)

		if err != nil {

			return nil, GetAuditSummaryResult{}, fmt.Errorf("get_audit_summary: %w", err)

		}

		if summary.RiskyEvents == nil {

			summary.RiskyEvents = []coreaudit.AuditEvent{}

		}

		return nil, GetAuditSummaryResult{WindowHours: hours, Summary: summary}, nil

	}

}


