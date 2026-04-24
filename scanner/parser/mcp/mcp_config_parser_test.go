// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp_test

import (
	"testing"

	mcpparser "github.com/doc-scout/mcp-server/scanner/parser/mcp"
)

func TestMcpConfigParser_FileType(t *testing.T) {
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	if p.FileType() != "mcp-config" {
		t.Fatalf("want FileType=mcp-config, got %q", p.FileType())
	}
}

func TestMcpConfigParser_ParseDotMcpJSON(t *testing.T) {
	input := []byte(`{
		"mcpServers": {
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"]
			}
		}
	}`)

	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) != 1 {
		t.Fatalf("want 1 AuxEntity, got %d", len(result.AuxEntities))
	}
	ent := result.AuxEntities[0]
	if ent.Name != "github" {
		t.Fatalf("want name=github, got %q", ent.Name)
	}
	if ent.EntityType != "mcp-server" {
		t.Fatalf("want entityType=mcp-server, got %q", ent.EntityType)
	}
	hasTransport := false
	hasToolObs := false
	for _, obs := range ent.Observations {
		if obs == "transport:stdio" {
			hasTransport = true
		}
		if len(obs) > 5 && obs[:5] == "tool:" {
			hasToolObs = true
		}
	}
	if !hasTransport {
		t.Fatalf("expected transport:stdio observation, got: %v", ent.Observations)
	}
	if !hasToolObs {
		t.Fatalf("expected tool: observations from known registry, got: %v", ent.Observations)
	}
}

func TestMcpConfigParser_UnknownServerNoToolObs(t *testing.T) {
	input := []byte(`{
		"mcpServers": {
			"my-custom-server": {
				"command": "my-server",
				"args": ["--port", "3000"]
			}
		}
	}`)

	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) != 1 {
		t.Fatalf("want 1 AuxEntity, got %d", len(result.AuxEntities))
	}
	for _, obs := range result.AuxEntities[0].Observations {
		if len(obs) > 5 && obs[:5] == "tool:" {
			t.Fatalf("unexpected tool: observation for unknown server: %q", obs)
		}
	}
}

func TestMcpConfigParser_TransportInference(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		wantTrans string
	}{
		{
			name:      "stdio from command",
			input:     []byte(`{"mcpServers":{"srv":{"command":"node","args":["server.js"]}}}`),
			wantTrans: "stdio",
		},
		{
			name:      "http from url",
			input:     []byte(`{"mcpServers":{"srv":{"url":"http://localhost:3000/mcp"}}}`),
			wantTrans: "http",
		},
		{
			name:      "sse from explicit transport field",
			input:     []byte(`{"mcpServers":{"srv":{"url":"http://localhost:3000/sse","transport":"sse"}}}`),
			wantTrans: "sse",
		},
	}

	p := mcpparser.NewMcpConfigParser(mcpparser.KnownServerRegistry{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := p.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			found := false
			for _, obs := range result.AuxEntities[0].Observations {
				if obs == "transport:"+tc.wantTrans {
					found = true
				}
			}
			if !found {
				t.Fatalf("want transport:%s in observations, got: %v", tc.wantTrans, result.AuxEntities[0].Observations)
			}
		})
	}
}

func TestMcpConfigParser_MalformedJSON(t *testing.T) {
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	_, err := p.Parse([]byte(`{not valid json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestMcpConfigParser_EmptyMcpServers(t *testing.T) {
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse([]byte(`{"mcpServers":{}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) != 0 {
		t.Fatalf("want 0 AuxEntities for empty mcpServers, got %d", len(result.AuxEntities))
	}
}

func TestMcpConfigParser_UsesRelationEmitted(t *testing.T) {
	input := []byte(`{"mcpServers":{"github":{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}}}`)
	p := mcpparser.NewMcpConfigParser(mcpparser.DefaultKnownServers())
	result, err := p.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.AuxEntities) == 0 {
		t.Fatal("no AuxEntities")
	}
	// uses_mcp relations go in ParsedFile.Relations (not inside AuxEntity)
	found := false
	for _, rel := range result.Relations {
		if rel.RelationType == "uses_mcp" && rel.To == "github" && rel.From == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected uses_mcp relation with From='', To='github' in ParsedFile.Relations, got: %v", result.Relations)
	}
}
