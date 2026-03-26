package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"docscout-mcp/scanner"
)

// DocumentScanner defines the interface for interacting with the documentation scanner.
type DocumentScanner interface {
	ListRepos() []scanner.RepoInfo
	SearchDocs(query string) []scanner.FileEntry
	GetFileContent(ctx context.Context, repo string, path string) (string, error)
}

// Register adds all DocScout MCP tools to the server.
func Register(s *server.MCPServer, sc DocumentScanner) {
	// --- list_repos ---
	listReposTool := mcp.NewTool(
		"list_repos",
		mcp.WithDescription("Lists all repositories in the organization that contain documentation files (catalog-info.yaml, mkdocs.yml, openapi.yaml, swagger.json, README.md, docs/*.md)."),
	)
	s.AddTool(listReposTool, listReposHandler(sc))

	// --- search_docs ---
	searchDocsTool := mcp.NewTool(
		"search_docs",
		mcp.WithDescription("Searches for documentation files by matching a query term against file paths and repository names."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The search term to match against file paths and repo names."),
		),
	)
	s.AddTool(searchDocsTool, searchDocsHandler(sc))

	// --- get_file_content ---
	getFileContentTool := mcp.NewTool(
		"get_file_content",
		mcp.WithDescription("Retrieves the raw content of a specific documentation file from a GitHub repository. Note: For security reasons, this tool will only return files that have been successfully indexed as documentation (i.e. returned by list_repos or search_docs)."),
		mcp.WithString("repo",
			mcp.Required(),
			mcp.Description("The repository name (not full name, just the repo part)."),
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("The file path within the repository (e.g. 'docs/guide.md' or 'README.md')."),
		),
	)
	s.AddTool(getFileContentTool, getFileContentHandler(sc))
}

func listReposHandler(sc DocumentScanner) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repos := sc.ListRepos()

		// Build a summary for each repo.
		type repoSummary struct {
			Name        string   `json:"name"`
			FullName    string   `json:"full_name"`
			Description string   `json:"description"`
			URL         string   `json:"url"`
			FileCount   int      `json:"file_count"`
			FileTypes   []string `json:"file_types"`
		}

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

		data, err := json.MarshalIndent(summaries, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("error marshaling repos: %v", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func searchDocsHandler(sc DocumentScanner) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("parameter 'query' is required"), nil
		}

		results := sc.SearchDocs(query)

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("error marshaling results: %v", err)), nil
		}

		if len(results) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No documentation files found matching '%s'.", query)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func getFileContentHandler(sc DocumentScanner) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := request.RequireString("repo")
		if err != nil {
			return mcp.NewToolResultError("parameter 'repo' is required"), nil
		}
		path, err := request.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError("parameter 'path' is required"), nil
		}

		content, err := sc.GetFileContent(ctx, repo, path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
		}

		return mcp.NewToolResultText(content), nil
	}
}
