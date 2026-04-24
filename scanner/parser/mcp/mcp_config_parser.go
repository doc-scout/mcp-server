// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

// McpConfigParser discovers MCP server definitions from well-known config file formats
// and indexes each server as an mcp-server entity with a uses_mcp relation edge.
type McpConfigParser struct {
	known KnownServerRegistry
}

// NewMcpConfigParser creates a parser pre-loaded with the given known-server registry.
func NewMcpConfigParser(known KnownServerRegistry) *McpConfigParser {
	return &McpConfigParser{known: known}
}

func (p *McpConfigParser) FileType() string { return "mcp-config" }

func (p *McpConfigParser) Filenames() []string {
	return []string{
		".mcp.json",
		"mcp.json",
		".cursor/mcp.json",
		"claude_desktop_config.json",
		".vscode/mcp.json",
	}
}

// rawConfig is the shared envelope all MCP config formats use.
type rawConfig struct {
	McpServers map[string]rawServerEntry `json:"mcpServers"`
}

// rawServerEntry normalises across config formats.
type rawServerEntry struct {
	Command   string         `json:"command,omitzero"`
	Args      []string       `json:"args,omitzero"`
	URL       string         `json:"url,omitzero"`
	Transport string         `json:"transport,omitzero"`
	Tools     []rawToolEntry `json:"tools,omitzero"`
}

type rawToolEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitzero"`
}

func (p *McpConfigParser) Parse(data []byte) (parser.ParsedFile, error) {
	var cfg rawConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return parser.ParsedFile{}, fmt.Errorf("mcp-config: %w", err)
	}

	var auxEntities []parser.AuxEntity
	var relations []parser.ParsedRelation

	for name, entry := range cfg.McpServers {
		obs := p.buildObservations(name, entry)
		auxEntities = append(auxEntities, parser.AuxEntity{
			Name:         name,
			EntityType:   "mcp-server",
			Observations: obs,
		})
		// uses_mcp: From="" means indexer fills it with the repo service name
		relations = append(relations, parser.ParsedRelation{
			From:         "",
			To:           name,
			RelationType: "uses_mcp",
		})
	}

	return parser.ParsedFile{AuxEntities: auxEntities, Relations: relations}, nil
}

func (p *McpConfigParser) buildObservations(name string, entry rawServerEntry) []string {
	var obs []string

	transport := inferTransport(entry)
	obs = append(obs, "transport:"+transport)

	if entry.Command != "" {
		cmd := entry.Command
		if len(entry.Args) > 0 {
			cmd += " " + strings.Join(entry.Args, " ")
		}
		obs = append(obs, "command:"+cmd)
	}
	if entry.URL != "" {
		obs = append(obs, "url:"+entry.URL)
	}

	// Tool observations from inline config.
	for _, t := range entry.Tools {
		if t.Name != "" {
			obs = append(obs, "tool:"+t.Name+": "+t.Description)
		}
	}

	// Enrich from known registry (case-insensitive lookup).
	if knownTools := p.known.Lookup(name); len(knownTools) > 0 {
		obs = append(obs, knownTools...)
	}

	return obs
}

// inferTransport determines the transport type from the entry fields.
func inferTransport(entry rawServerEntry) string {
	if entry.Transport != "" {
		return strings.ToLower(entry.Transport)
	}
	if entry.URL != "" {
		if strings.Contains(entry.URL, "/sse") {
			return "sse"
		}
		return "http"
	}
	if entry.Command != "" {
		return "stdio"
	}
	return "unknown"
}
