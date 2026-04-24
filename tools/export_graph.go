// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doc-scout/mcp-server/memory"
)

// ExportGraphArgs defines input parameters for the export_graph tool.

type ExportGraphArgs struct {
	Format string `json:"format,omitempty"      jsonschema:"Export format: 'html' (interactive force graph, offline) or 'json' (nodes+edges). Defaults to 'html'."`

	Title string `json:"title,omitempty"       jsonschema:"Title shown in the exported artifact (e.g. org name). Defaults to 'Knowledge Graph'."`

	OutputPath string `json:"output_path,omitempty" jsonschema:"Absolute path where the file will be written (e.g. /tmp/graph.html). If omitted the content is returned inline."`
}

// ExportGraphResult is the structured output of the export_graph tool.

type ExportGraphResult struct {
	Format string `json:"format"`

	EntityCount int `json:"entity_count"`

	EdgeCount int `json:"edge_count"`

	OutputPath string `json:"output_path,omitempty"`

	Content string `json:"content,omitempty"`
}

func exportGraphHandler(g GraphStore) func(ctx context.Context, req *mcp.CallToolRequest, args ExportGraphArgs) (*mcp.CallToolResult, ExportGraphResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args ExportGraphArgs) (*mcp.CallToolResult, ExportGraphResult, error) {

		format := strings.ToLower(strings.TrimSpace(args.Format))

		if format == "" {

			format = "html"

		}

		if format != "html" && format != "json" {

			return nil, ExportGraphResult{}, fmt.Errorf("unsupported format %q: use 'html' or 'json'", format)

		}

		title := strings.TrimSpace(args.Title)

		if title == "" {

			title = "Knowledge Graph"

		}

		kg, err := g.ReadGraph()

		if err != nil {

			return nil, ExportGraphResult{}, fmt.Errorf("export_graph: failed to read graph: %w", err)

		}

		data, err := memory.ExportGraph(kg, format, title)

		if err != nil {

			return nil, ExportGraphResult{}, fmt.Errorf("export_graph: render failed: %w", err)

		}

		result := ExportGraphResult{

			Format: format,

			EntityCount: len(kg.Entities),

			EdgeCount: len(kg.Relations),
		}

		if path := strings.TrimSpace(args.OutputPath); path != "" {

			if err := os.WriteFile(path, data, 0644); err != nil {

				return nil, ExportGraphResult{}, fmt.Errorf("export_graph: write to %q failed: %w", path, err)

			}

			result.OutputPath = path

		} else {

			result.Content = string(data)

		}

		return nil, result, nil

	}

}
