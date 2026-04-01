// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetFileContentArgs struct {
	Repo string `json:"repo" jsonschema:"The repository name, MUST be in the format 'owner/repo' as returned by list_repos (e.g. 'my-org/my-service')."`
	Path string `json:"path" jsonschema:"The file path within the repository (e.g. 'docs/guide.md' or 'README.md')."`
}

type RawContentResult struct {
	Content string `json:"content" jsonschema:"The raw text content of the file."`
}

func getFileContentHandler(sc DocumentScanner, docMetrics *DocMetrics) func(ctx context.Context, req *mcp.CallToolRequest, args GetFileContentArgs) (*mcp.CallToolResult, RawContentResult, error) {
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

		docMetrics.Record(args.Repo, args.Path)
		return nil, RawContentResult{Content: content}, nil
	}
}
