// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"

	"gorm.io/gorm"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
	infradb "github.com/doc-scout/mcp-server/internal/infra/db"
)

func setupTestDB(t *testing.T) (*coregraph.MemoryService, *gorm.DB) {
	t.Helper()
	database, err := infradb.OpenDB("")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	graphRepo := infradb.NewGraphRepo(database)
	svc := coregraph.NewMemoryService(graphRepo)
	_, err = svc.CreateEntities([]coregraph.Entity{
		{Name: "svc-a", EntityType: "service"},
		{Name: "svc-b", EntityType: "service"},
		{Name: "api-v1", EntityType: "api"},
	})
	if err != nil {
		t.Fatalf("create entities: %v", err)
	}
	_, err = svc.CreateRelations([]coregraph.Relation{
		{From: "svc-a", To: "svc-b", RelationType: "calls_service", Confidence: "authoritative"},
		{From: "svc-a", To: "api-v1", RelationType: "exposes_api", Confidence: "inferred"},
	})
	if err != nil {
		t.Fatalf("create relations: %v", err)
	}
	return svc, database
}

func TestNodeID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"svc-a", "n_svc_a"},
		{"api/v2", "n_api_v2"},
		{"123start", "n_123start"},
		{"simple", "n_simple"},
		{"has space", "n_has_space"},
	}
	for _, c := range cases {
		if got := nodeID(c.in); got != c.want {
			t.Errorf("nodeID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMermaidShape(t *testing.T) {
	cases := []struct {
		name, typ, want string
	}{
		{"svc-a", "service", `n_svc_a["svc-a\nservice"]`},
		{"api-v1", "api", `n_api_v1(["api-v1\napi"])`},
		{"squad-x", "team", `n_squad_x(("squad-x\nteam"))`},
		{"rpc-svc", "grpc-service", `n_rpc_svc[("rpc-svc\ngrpc-service")]`},
		{"other", "custom", `n_other["other\ncustom"]`},
	}
	for _, c := range cases {
		if got := mermaidShape(c.name, c.typ); got != c.want {
			t.Errorf("mermaidShape(%q,%q) = %q, want %q", c.name, c.typ, got, c.want)
		}
	}
}

func TestBuildPie(t *testing.T) {
	counts := map[string]int64{"service": 5, "api": 3}
	out := buildPie(counts)
	if !strings.Contains(out, "```mermaid") {
		t.Error("missing mermaid fence")
	}
	if !strings.Contains(out, `"service" : 5`) {
		t.Error("missing service count")
	}
	if !strings.Contains(out, `"api" : 3`) {
		t.Error("missing api count")
	}
}

func TestBuildPieEmpty(t *testing.T) {
	out := buildPie(nil)
	if out != "" {
		t.Errorf("expected empty string for nil counts, got %q", out)
	}
}

func TestBuildFlowchart(t *testing.T) {
	nodes := []TopNode{
		{Name: "svc-a", EntityType: "service"},
		{Name: "svc-b", EntityType: "service"},
	}
	edges := []TopEdge{
		{From: "svc-a", To: "svc-b", RelationType: "calls_service", Confidence: "authoritative"},
	}
	out := buildFlowchart(nodes, edges, 2, 1)
	if !strings.Contains(out, "```mermaid") {
		t.Error("missing mermaid fence")
	}
	if !strings.Contains(out, "graph LR") {
		t.Error("missing graph LR")
	}
	if !strings.Contains(out, `-->|calls_service|`) {
		t.Error("missing edge label")
	}
}

func TestBuildFlowchartInferred(t *testing.T) {
	nodes := []TopNode{{Name: "a", EntityType: "service"}, {Name: "b", EntityType: "api"}}
	edges := []TopEdge{{From: "a", To: "b", RelationType: "exposes_api", Confidence: "inferred"}}
	out := buildFlowchart(nodes, edges, 2, 1)
	if !strings.Contains(out, "-.->") {
		t.Error("inferred edge should use dashed arrow")
	}
}

func TestBuildFlowchartTruncation(t *testing.T) {
	nodes := []TopNode{{Name: "a", EntityType: "service"}}
	edges := []TopEdge{}
	out := buildFlowchart(nodes, edges, 10, 5)
	if !strings.Contains(out, "⚠️") {
		t.Error("expected truncation warning")
	}
	if !strings.Contains(out, "10") {
		t.Error("expected total entity count in warning")
	}
}

func TestBuildFlowchartEmpty(t *testing.T) {
	out := buildFlowchart(nil, nil, 0, 0)
	if out != "" {
		t.Errorf("expected empty for nil nodes, got %q", out)
	}
}

func TestQueryTopNodes(t *testing.T) {
	_, database := setupTestDB(t)
	nodes, total, err := queryTopNodes(database, 10)
	if err != nil {
		t.Fatalf("queryTopNodes: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(nodes) == 0 {
		t.Fatal("expected nodes")
	}
	if nodes[0].Name != "svc-a" {
		t.Errorf("first node = %q, want svc-a", nodes[0].Name)
	}
}

func TestQueryEdges(t *testing.T) {
	_, database := setupTestDB(t)
	nodes, _, _ := queryTopNodes(database, 10)
	edges, total, err := queryEdges(database, nodes, 10)
	if err != nil {
		t.Fatalf("queryEdges: %v", err)
	}
	if total != 2 {
		t.Errorf("total edges = %d, want 2", total)
	}
	if len(edges) != 2 {
		t.Errorf("edges len = %d, want 2", len(edges))
	}
}

func TestGenerateReport(t *testing.T) {
	svc, database := setupTestDB(t)
	out, err := GenerateReport(svc, database, ReportConfig{
		Repo:     "org/repo",
		Elapsed:  42,
		MaxNodes: 20,
		MaxEdges: 40,
	})
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if !strings.Contains(out, "## DocScout Graph Analysis") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "org/repo") {
		t.Error("missing repo name")
	}
	if !strings.Contains(out, "42s") {
		t.Error("missing elapsed")
	}
	if !strings.Contains(out, "pie title") {
		t.Error("missing pie chart")
	}
	if !strings.Contains(out, "graph LR") {
		t.Error("missing flowchart")
	}
}

func TestGenerateReportEmptyDB(t *testing.T) {
	database, _ := infradb.OpenDB("")
	graphRepo := infradb.NewGraphRepo(database)
	svc := coregraph.NewMemoryService(graphRepo)
	out, err := GenerateReport(svc, database, ReportConfig{Repo: "org/repo", MaxNodes: 20, MaxEdges: 40})
	if err != nil {
		t.Fatalf("GenerateReport empty: %v", err)
	}
	if !strings.Contains(out, "## DocScout Graph Analysis") {
		t.Error("missing header even on empty graph")
	}
}
