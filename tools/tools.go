// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docscout-mcp/scanner"
)

// DocumentScanner defines the interface for interacting with the documentation scanner.
type DocumentScanner interface {
	ListRepos() []scanner.RepoInfo
	SearchDocs(query string) []scanner.FileEntry
	GetFileContent(ctx context.Context, repo string, path string) (string, error)
}

// Register adds all DocScout MCP tools to the server.
func Register(s *mcp.Server, sc DocumentScanner) {
	// --- list_repos ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_repos",
		Description: "Lists all repositories in the organization that contain documentation files (catalog-info.yaml, mkdocs.yml, openapi.yaml, swagger.json, README.md, docs/*.md).",
	}, listReposHandler(sc))

	// --- search_docs ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_docs",
		Description: "Searches for documentation files by matching a query term against file paths and repository names.",
	}, searchDocsHandler(sc))

	// --- get_file_content ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_content",
		Description: "Retrieves the raw content of a specific documentation file from a GitHub repository. Note: For security reasons, this tool will only return files that have been successfully indexed as documentation (i.e. returned by list_repos or search_docs).",
	}, getFileContentHandler(sc))
}

// --- Handler Implementations ---

// repoSummary is returned by list_repos
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
		// Returning the structured struct directly allows the official SDK to infer its JSON schema and auto-marshal!
		return nil, ListReposResult{Repos: summaries}, nil
	}
}

// SearchDocsArgs describes the input to the search_docs tool.
type SearchDocsArgs struct {
	Query string `json:"query" jsonschema:"The search term to match against file paths and repo names."`
}
type SearchDocsResult struct {
	Files []scanner.FileEntry `json:"files" jsonschema:"List of files matching the query"`
}

func searchDocsHandler(sc DocumentScanner) func(ctx context.Context, req *mcp.CallToolRequest, args SearchDocsArgs) (*mcp.CallToolResult, SearchDocsResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchDocsArgs) (*mcp.CallToolResult, SearchDocsResult, error) {
		if args.Query == "" {
			return nil, SearchDocsResult{}, fmt.Errorf("parameter 'query' is required")
		}
		results := sc.SearchDocs(args.Query)
		return nil, SearchDocsResult{Files: results}, nil
	}
}

// GetFileContentArgs describes the input to the get_file_content tool.
type GetFileContentArgs struct {
	Repo string `json:"repo" jsonschema:"The repository name (not full name, just the repo part)."`
	Path string `json:"path" jsonschema:"The file path within the repository (e.g. 'docs/guide.md' or 'README.md')."`
}

// RawContentResult holds the string content returned.
type RawContentResult struct {
	Content string `json:"content" jsonschema:"The raw text content of the file."`
}

func getFileContentHandler(sc DocumentScanner) func(ctx context.Context, req *mcp.CallToolRequest, args GetFileContentArgs) (*mcp.CallToolResult, RawContentResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args GetFileContentArgs) (*mcp.CallToolResult, RawContentResult, error) {
		if args.Repo == "" {
			return nil, RawContentResult{}, fmt.Errorf("parameter 'repo' is required")
		}
		if args.Path == "" {
			return nil, RawContentResult{}, fmt.Errorf("parameter 'path' is required")
		}

		content, err := sc.GetFileContent(ctx, args.Repo, args.Path)
		if err != nil {
			return nil, RawContentResult{}, fmt.Errorf("error: %v", err)
		}

		return nil, RawContentResult{Content: content}, nil
	}
}
