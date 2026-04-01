// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchContentArgs struct {
	Query string `json:"query" jsonschema:"The term to search for inside documentation content. Use natural language terms like 'payment', 'authentication', 'event sourcing'."`
	Repo  string `json:"repo,omitempty" jsonschema:"Optional: filter results to a single repository name (e.g. 'org/payment-service')."`
}

type SearchContentResult struct {
	Matches []memory.ContentMatch `json:"matches" jsonschema:"List of files containing the query term, with a snippet showing the matched context."`
}

func searchContentHandler(search ContentSearcher, docMetrics *DocMetrics) func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {
		if strings.TrimSpace(args.Query) == "" {
			return nil, SearchContentResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")
		}
		matches, err := search.Search(args.Query, args.Repo)
		if err != nil {
			return nil, SearchContentResult{}, err
		}
		for _, m := range matches {
			docMetrics.Record(m.RepoName, m.Path)
		}
		return nil, SearchContentResult{Matches: matches}, nil
	}
}
