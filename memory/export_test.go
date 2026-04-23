// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/doc-scout/mcp-server/memory"
)

func testGraph() memory.KnowledgeGraph {
	return memory.KnowledgeGraph{
		Entities: []memory.Entity{
			{Name: "payment-service", EntityType: "service", Observations: []string{"go_version:1.26"}},
			{Name: "billing-service", EntityType: "service", Observations: []string{"go_version:1.24"}},
			{Name: "payments-team", EntityType: "team", Observations: []string{}},
		},
		Relations: []memory.Relation{
			{From: "payment-service", To: "billing-service", RelationType: "depends_on"},
			{From: "payments-team", To: "payment-service", RelationType: "owns"},
		},
	}
}

func TestExportGraphJSON(t *testing.T) {
	kg := testGraph()
	data, err := memory.ExportGraph(kg, "json", "test-org")
	if err != nil {
		t.Fatalf("ExportGraph json: %v", err)
	}
	var out struct {
		Nodes []map[string]any `json:"nodes"`
		Edges []map[string]any `json:"edges"`
		Title string           `json:"title"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Nodes) != 3 {
		t.Errorf("want 3 nodes, got %d", len(out.Nodes))
	}
	if len(out.Edges) != 2 {
		t.Errorf("want 2 edges, got %d", len(out.Edges))
	}
	if out.Title != "test-org" {
		t.Errorf("want title 'test-org', got %q", out.Title)
	}
}

func TestExportGraphHTML(t *testing.T) {
	kg := testGraph()
	data, err := memory.ExportGraph(kg, "html", "test-org")
	if err != nil {
		t.Fatalf("ExportGraph html: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(html, "payment-service") {
		t.Error("missing node name in HTML")
	}
	if !strings.Contains(html, "depends_on") {
		t.Error("missing relation type in HTML")
	}
}

func TestExportGraphUnknownFormat(t *testing.T) {
	_, err := memory.ExportGraph(testGraph(), "pdf", "x")
	if err == nil {
		t.Error("expected error for unknown format")
	}
}
