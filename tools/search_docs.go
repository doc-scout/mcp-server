// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/leonancarvalho/docscout-mcp/scanner"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchDocsArgs struct {
	Query string `json:"query" jsonschema:"The search term to match against file paths and repo names."`
}
type SearchDocsResult struct {
	Files []scanner.FileEntry `json:"files" jsonschema:"List of files matching the query"`
}

func searchDocsHandler(sc DocumentScanner) func(ctx context.Context, req *mcp.CallToolRequest, args SearchDocsArgs) (*mcp.CallToolResult, SearchDocsResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchDocsArgs) (*mcp.CallToolResult, SearchDocsResult, error) {
		if strings.TrimSpace(args.Query) == "" {
			return nil, SearchDocsResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")
		}
		results := sc.SearchDocs(args.Query)
		return nil, SearchDocsResult{Files: results}, nil
	}
}
