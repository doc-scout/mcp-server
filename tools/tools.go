// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/leonancarvalho/docscout-mcp/scanner"
)

// DocumentScanner defines the interface for interacting with the documentation scanner.
type DocumentScanner interface {
	ListRepos() []scanner.RepoInfo
	SearchDocs(query string) []scanner.FileEntry
	GetFileContent(ctx context.Context, repo string, path string) (string, error)
	Status() (scanning bool, lastScan time.Time, repoCount int)
}

// GraphCounter provides the entity count for get_scan_status.
type GraphCounter interface {
	EntityCount() (int64, error)
}

// ContentSearcher provides full-text search over cached documentation content.
type ContentSearcher interface {
	Search(query, repo string) ([]memory.ContentMatch, error)
	Count() (int64, error)
}

// withRecovery wraps an MCP tool handler to catch and log panics gracefully.
func withRecovery[A, R any](
	name string,
	handler func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error),
) func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args A) (res *mcp.CallToolResult, ret R, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[tools] MCP tool panicked: tool=%s panic=%v\nstack=%s", name, r, string(debug.Stack()))
				err = fmt.Errorf("internal server error in tool '%s' (panic recovered: %v)", name, r)
			}
		}()
		return handler(ctx, req, args)
	}
}

// Register adds all DocScout MCP tools to the server.
// graph and search may be nil — get_scan_status degrades gracefully, search_content is omitted.
func Register(s *mcp.Server, sc DocumentScanner, graph GraphCounter, search ContentSearcher) {
	// --- list_repos ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_repos",
		Description: "Lists all repositories in the organization that contain documentation files (catalog-info.yaml, mkdocs.yml, openapi.yaml, swagger.json, README.md, docs/*.md).",
	}, withRecovery("list_repos", listReposHandler(sc)))

	// --- search_docs ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_docs",
		Description: "Searches for documentation files by matching a query term against file paths and repository names.",
	}, withRecovery("search_docs", searchDocsHandler(sc)))

	// --- get_file_content ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_content",
		Description: "Retrieves the raw content of a specific documentation file from a GitHub repository. Note: For security reasons, this tool will only return files that have been successfully indexed as documentation (i.e. returned by list_repos or search_docs).",
	}, withRecovery("get_file_content", getFileContentHandler(sc)))

	// --- get_scan_status ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_scan_status",
		Description: "Returns the current state of the documentation scanner and knowledge graph index. Call this before searching to confirm the index is populated, especially right after startup.",
	}, withRecovery("get_scan_status", getScanStatusHandler(sc, graph, search)))

	// --- search_content (only when content caching is enabled) ---
	if search != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "search_content",
			Description: "Full-text search across the content of all cached documentation files. Use this to find which service handles a specific responsibility (e.g. 'payment', 'authentication'). Only available when SCAN_CONTENT=true.",
		}, withRecovery("search_content", searchContentHandler(search)))
	}
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
		if strings.TrimSpace(args.Query) == "" {
			return nil, SearchDocsResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")
		}
		results := sc.SearchDocs(args.Query)
		return nil, SearchDocsResult{Files: results}, nil
	}
}

// GetFileContentArgs describes the input to the get_file_content tool.
type GetFileContentArgs struct {
	Repo string `json:"repo" jsonschema:"The repository name, MUST be in the format 'owner/repo' as returned by list_repos (e.g. 'my-org/my-service')."`
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

// --- get_scan_status ---

type ScanStatusArgs struct{}

type ScanStatusResult struct {
	Scanning       bool      `json:"scanning"`
	LastScanAt     time.Time `json:"last_scan_at"`
	RepoCount      int       `json:"repo_count"`
	ContentIndexed int64     `json:"content_indexed"`
	GraphEntities  int64     `json:"graph_entities"`
	ContentEnabled bool      `json:"content_enabled"`
}

func getScanStatusHandler(sc DocumentScanner, graph GraphCounter, search ContentSearcher) func(ctx context.Context, req *mcp.CallToolRequest, args ScanStatusArgs) (*mcp.CallToolResult, ScanStatusResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args ScanStatusArgs) (*mcp.CallToolResult, ScanStatusResult, error) {
		scanning, lastScan, repoCount := sc.Status()

		var graphEntities int64
		if graph != nil {
			graphEntities, _ = graph.EntityCount()
		}

		var contentIndexed int64
		contentEnabled := search != nil
		if search != nil {
			contentIndexed, _ = search.Count()
		}

		return nil, ScanStatusResult{
			Scanning:       scanning,
			LastScanAt:     lastScan,
			RepoCount:      repoCount,
			ContentIndexed: contentIndexed,
			GraphEntities:  graphEntities,
			ContentEnabled: contentEnabled,
		}, nil
	}
}

// --- search_content ---

type SearchContentArgs struct {
	Query string `json:"query" jsonschema:"The term to search for inside documentation content. Use natural language terms like 'payment', 'authentication', 'event sourcing'."`
	Repo  string `json:"repo,omitempty" jsonschema:"Optional: filter results to a single repository name (e.g. 'org/payment-service')."`
}

type SearchContentResult struct {
	Matches []memory.ContentMatch `json:"matches" jsonschema:"List of files containing the query term, with a snippet showing the matched context."`
}

func searchContentHandler(search ContentSearcher) func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchContentArgs) (*mcp.CallToolResult, SearchContentResult, error) {
		if strings.TrimSpace(args.Query) == "" {
			return nil, SearchContentResult{}, fmt.Errorf("parameter 'query' must not be empty or whitespace-only")
		}
		matches, err := search.Search(args.Query, args.Repo)
		if err != nil {
			return nil, SearchContentResult{}, err
		}
		return nil, SearchContentResult{Matches: matches}, nil
	}
}
