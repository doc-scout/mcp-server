// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type repoSummary struct {
	Name        string   `json:"name"`
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	FileCount   int      `json:"file_count"`
	FileTypes   []string `json:"file_types"`
}

type ListReposArgs struct{}
type ListReposResult struct {
	Repos []repoSummary `json:"repos" jsonschema:"List of repositories with documentation"`
}

func listReposHandler(sc DocumentScanner) func(ctx context.Context, req *mcp.CallToolRequest, args ListReposArgs) (*mcp.CallToolResult, ListReposResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args ListReposArgs) (*mcp.CallToolResult, ListReposResult, error) {
		repos := sc.ListRepos()
		summaries := make([]repoSummary, 0, len(repos))
		for _, r := range repos {
			types := make(map[string]bool)
			for _, f := range r.Files {
				types[f.Type] = true
			}
			typeList := make([]string, 0, len(types))
			for t := range types {
				typeList = append(typeList, t)
			}
			summaries = append(summaries, repoSummary{
				Name:        r.Name,
				FullName:    r.FullName,
				Description: r.Description,
				URL:         r.HTMLURL,
				FileCount:   len(r.Files),
				FileTypes:   typeList,
			})
		}
		return nil, ListReposResult{Repos: summaries}, nil
	}
}
