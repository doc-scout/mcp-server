# Mermaid Graph Report Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `go run ./cmd/report` command that generates a rich Markdown+Mermaid report from the DocScout SQLite database, replacing the simple counter table in the GitHub Actions Step Summary and PR comment.

**Architecture:** A new `cmd/report/` package opens the existing SQLite database via `memory.OpenDB`, runs two custom GORM raw queries (top-N by connectivity, edges between those nodes), and emits Markdown with two Mermaid blocks to stdout. `bin/run-scan.sh` captures that output and writes it to both the Step Summary and PR comment. `action.yml` gains two new inputs (`max_nodes`, `max_edges`) and a `setup-go` step.

**Tech Stack:** Go 1.26, GORM (`gorm.io/gorm`), `github.com/doc-scout/mcp-server/memory`, standard `flag` package, `strings.Builder`, `regexp`.

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `cmd/report/report.go` | Create | All query + generation logic (exported functions for testing) |
| `cmd/report/main.go` | Create | Flag parsing + wiring only |
| `cmd/report/report_test.go` | Create | Unit tests using in-memory SQLite |
| `action.yml` | Modify | Add `max_nodes`, `max_edges` inputs; add `setup-go` step |
| `bin/run-scan.sh` | Modify | Replace summary/comment generation with `go run ./cmd/report` |

---

### Task 1: Create `cmd/report/report.go` with node sanitization and shape mapping

**Files:**
- Create: `cmd/report/report.go`
- Create: `cmd/report/report_test.go`

- [ ] **Step 1: Write failing tests for `nodeID` and `mermaidShape`**

Create `cmd/report/report_test.go`:

```go
package report

import (
	"testing"
)

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
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... 2>&1 | head -20
```
Expected: compilation error — package does not exist yet.

- [ ] **Step 3: Create `cmd/report/report.go` with the two functions**

```go
package report

import (
	"fmt"
	"regexp"
	"strings"
)

var reNonWord = regexp.MustCompile(`[^a-zA-Z0-9]`)

// nodeID converts an entity name into a valid Mermaid node identifier.
func nodeID(name string) string {
	id := reNonWord.ReplaceAllString(name, "_")
	return "n_" + id
}

// mermaidShape returns a Mermaid node declaration with shape based on entity type.
func mermaidShape(name, entityType string) string {
	id := nodeID(name)
	label := fmt.Sprintf("%s\\n%s", name, entityType)
	switch entityType {
	case "api":
		return fmt.Sprintf(`%s(["%s"])`, id, label)
	case "team":
		return fmt.Sprintf(`%s(("%s"))`, id, label)
	case "grpc-service":
		return fmt.Sprintf(`%s[("%s")]`, id, label)
	default: // "service" and everything else
		return fmt.Sprintf(`%s["%s"]`, id, label)
	}
}

// arrowStyle returns the Mermaid arrow for a relation confidence value.
func arrowStyle(confidence string) string {
	if confidence == "inferred" {
		return "-.->"
	}
	return "-->"
}

// sanitizeLabel removes characters that would break a Mermaid edge label.
func sanitizeLabel(s string) string {
	return strings.NewReplacer("|", "/", `"`, "'", "\n", " ").Replace(s)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestNodeID|TestMermaidShape" -v 2>&1
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd E:/DEV/mcpdocs && rtk git add cmd/report/report.go cmd/report/report_test.go && rtk git commit -m "feat(report): add nodeID, mermaidShape, arrowStyle helpers"
```

---

### Task 2: Add pie chart generation

**Files:**
- Modify: `cmd/report/report.go`
- Modify: `cmd/report/report_test.go`

- [ ] **Step 1: Write failing test for `buildPie`**

Append to `cmd/report/report_test.go`:

```go
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
```

Add `"strings"` to imports if not already present (it is).

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestBuildPie" -v 2>&1
```
Expected: compile error — `buildPie` undefined.

- [ ] **Step 3: Implement `buildPie` in `report.go`**

Append to `cmd/report/report.go` (add `"fmt"`, `"sort"`, `"strings"` to imports — `fmt` is already there):

```go
// buildPie generates a Mermaid pie chart block from entity type counts.
// Returns empty string when counts is nil or empty.
func buildPie(counts map[string]int64) string {
	if len(counts) == 0 {
		return ""
	}
	// Sort by count descending for deterministic output.
	type kv struct {
		k string
		v int64
	}
	pairs := make([]kv, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})

	var b strings.Builder
	b.WriteString("```mermaid\npie title Entity Distribution\n")
	for _, p := range pairs {
		fmt.Fprintf(&b, "    %q : %d\n", p.k, p.v)
	}
	b.WriteString("```")
	return b.String()
}
```

Add `"sort"` to the import block.

- [ ] **Step 4: Run test to confirm it passes**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestBuildPie" -v 2>&1
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd E:/DEV/mcpdocs && rtk git add cmd/report/report.go cmd/report/report_test.go && rtk git commit -m "feat(report): add buildPie mermaid chart generation"
```

---

### Task 3: Add flowchart generation

**Files:**
- Modify: `cmd/report/report.go`
- Modify: `cmd/report/report_test.go`

- [ ] **Step 1: Write failing tests for `buildFlowchart`**

Append to `cmd/report/report_test.go`:

```go
func TestBuildFlowchart(t *testing.T) {
	nodes := []TopNode{
		{Name: "svc-a", EntityType: "service"},
		{Name: "svc-b", EntityType: "service"},
	}
	edges := []TopEdge{
		{From: "svc-a", To: "svc-b", RelationType: "calls_service", Confidence: "authoritative"},
	}
	out := buildFlowchart(nodes, edges, 20, 40, 2, 1)
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
	out := buildFlowchart(nodes, edges, 20, 40, 2, 1)
	if !strings.Contains(out, "-.->") {
		t.Error("inferred edge should use dashed arrow")
	}
}

func TestBuildFlowchartTruncation(t *testing.T) {
	nodes := []TopNode{{Name: "a", EntityType: "service"}}
	edges := []TopEdge{}
	out := buildFlowchart(nodes, edges, 1, 40, 10, 5)
	if !strings.Contains(out, "⚠️") {
		t.Error("expected truncation warning")
	}
	if !strings.Contains(out, "10") {
		t.Error("expected total entity count in warning")
	}
}

func TestBuildFlowchartEmpty(t *testing.T) {
	out := buildFlowchart(nil, nil, 20, 40, 0, 0)
	if out != "" {
		t.Errorf("expected empty for nil nodes, got %q", out)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestBuildFlowchart" -v 2>&1
```
Expected: compile error — `TopNode`, `TopEdge`, `buildFlowchart` undefined.

- [ ] **Step 3: Implement `TopNode`, `TopEdge`, `buildFlowchart` in `report.go`**

Append to `cmd/report/report.go`:

```go
// TopNode is an entity with its connectivity degree, returned by the top-N query.
type TopNode struct {
	Name       string
	EntityType string
	Degree     int
}

// TopEdge is a relation between two top nodes.
type TopEdge struct {
	From         string
	To           string
	RelationType string
	Confidence   string
}

// buildFlowchart generates a Mermaid graph LR block.
// totalNodes/totalEdges are the untruncated counts used for the warning line.
// Returns empty string when nodes is nil or empty.
func buildFlowchart(nodes []TopNode, edges []TopEdge, maxNodes, maxEdges, totalNodes, totalEdges int) string {
	if len(nodes) == 0 {
		return ""
	}

	var b strings.Builder

	truncated := len(nodes) < totalNodes || len(edges) < totalEdges
	if truncated {
		fmt.Fprintf(&b, "> ⚠️ Showing %d/%d entities · %d/%d relations — top by connectivity\n\n",
			len(nodes), totalNodes, len(edges), totalEdges)
	}

	b.WriteString("```mermaid\ngraph LR\n")
	for _, n := range nodes {
		fmt.Fprintf(&b, "    %s\n", mermaidShape(n.Name, n.EntityType))
	}
	for _, e := range edges {
		arrow := arrowStyle(e.Confidence)
		label := sanitizeLabel(e.RelationType)
		fmt.Fprintf(&b, "    %s %s|%s| %s\n",
			nodeID(e.From), arrow, label, nodeID(e.To))
	}
	b.WriteString("```")
	return b.String()
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestBuildFlowchart" -v 2>&1
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd E:/DEV/mcpdocs && rtk git add cmd/report/report.go cmd/report/report_test.go && rtk git commit -m "feat(report): add buildFlowchart with truncation and confidence arrows"
```

---

### Task 4: Add DB queries

**Files:**
- Modify: `cmd/report/report.go`
- Modify: `cmd/report/report_test.go`

- [ ] **Step 1: Write failing test for `queryTopNodes` and `queryEdges`**

Append to `cmd/report/report_test.go`:

```go
import (
	"testing"
	"strings"

	"github.com/doc-scout/mcp-server/memory"
)

func setupTestDB(t *testing.T) *memory.MemoryService {
	t.Helper()
	db, err := memory.OpenDB("")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	svc := memory.NewMemoryService(db)
	_, err = svc.CreateEntities([]memory.Entity{
		{Name: "svc-a", EntityType: "service"},
		{Name: "svc-b", EntityType: "service"},
		{Name: "api-v1", EntityType: "api"},
	})
	if err != nil {
		t.Fatalf("create entities: %v", err)
	}
	_, err = svc.CreateRelations([]memory.Relation{
		{From: "svc-a", To: "svc-b", RelationType: "calls_service", Confidence: "authoritative"},
		{From: "svc-a", To: "api-v1", RelationType: "exposes_api", Confidence: "inferred"},
	})
	if err != nil {
		t.Fatalf("create relations: %v", err)
	}
	return svc
}

func TestQueryTopNodes(t *testing.T) {
	svc := setupTestDB(t)
	nodes, total, err := queryTopNodes(svc, 10)
	if err != nil {
		t.Fatalf("queryTopNodes: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(nodes) == 0 {
		t.Fatal("expected nodes")
	}
	// svc-a has degree 2 (2 out), should be first
	if nodes[0].Name != "svc-a" {
		t.Errorf("first node = %q, want svc-a", nodes[0].Name)
	}
}

func TestQueryEdges(t *testing.T) {
	svc := setupTestDB(t)
	nodes, _, _ := queryTopNodes(svc, 10)
	edges, total, err := queryEdges(svc, nodes, 10)
	if err != nil {
		t.Fatalf("queryEdges: %v", err)
	}
	if total != 2 {
		t.Errorf("total edges = %d, want 2", total)
	}
	if len(edges) != 2 {
		t.Errorf("edges len = %d, want 2", len(edges))
	}
	_ = edges
}
```

Note: the test file already has the `testing` and `strings` imports from Task 1. Update the import block to be a single merged block:

```go
import (
	"strings"
	"testing"

	"github.com/doc-scout/mcp-server/memory"
)
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestQuery" -v 2>&1
```
Expected: compile error — `queryTopNodes`, `queryEdges` undefined; `memory` import not yet in report.go.

- [ ] **Step 3: Implement `queryTopNodes` and `queryEdges` in `report.go`**

Add to `cmd/report/report.go` imports: `"gorm.io/gorm"` and `"github.com/doc-scout/mcp-server/memory"`.

Append to `cmd/report/report.go`:

```go
// dbNode is the raw row returned by the connectivity query.
type dbNode struct {
	Name       string
	EntityType string `gorm:"column:entity_type"`
	Degree     int
}

// dbEdge is the raw row returned by the edge query.
type dbEdge struct {
	FromNode     string `gorm:"column:from_node"`
	ToNode       string `gorm:"column:to_node"`
	RelationType string `gorm:"column:relation_type"`
	Confidence   string
}

// queryTopNodes returns the top maxNodes entities ranked by connectivity degree,
// plus the total entity count before truncation.
func queryTopNodes(svc *memory.MemoryService, maxNodes int) ([]TopNode, int, error) {
	db := svc.DB()

	var totalCount int64
	if err := db.Table("db_entities").Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var rows []dbNode
	err := db.Raw(`
		SELECT e.name, e.entity_type,
		  (SELECT COUNT(*) FROM db_relations WHERE from_node = e.name) +
		  (SELECT COUNT(*) FROM db_relations WHERE to_node   = e.name) AS degree
		FROM db_entities e
		ORDER BY degree DESC
		LIMIT ?`, maxNodes).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	nodes := make([]TopNode, len(rows))
	for i, r := range rows {
		nodes[i] = TopNode{Name: r.Name, EntityType: r.EntityType, Degree: r.Degree}
	}
	return nodes, int(totalCount), nil
}

// queryEdges returns edges between the given nodes (both endpoints in the set),
// up to maxEdges, plus the total edge count in the subgraph before truncation.
func queryEdges(svc *memory.MemoryService, nodes []TopNode, maxEdges int) ([]TopEdge, int, error) {
	if len(nodes) == 0 {
		return nil, 0, nil
	}
	db := svc.DB()

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}

	var totalCount int64
	if err := db.Table("db_relations").
		Where("from_node IN ? AND to_node IN ?", names, names).
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	var rows []dbEdge
	err := db.Raw(`
		SELECT from_node, to_node, relation_type, confidence
		FROM db_relations
		WHERE from_node IN ? AND to_node IN ?
		LIMIT ?`, names, names, maxEdges).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	edges := make([]TopEdge, len(rows))
	for i, r := range rows {
		edges[i] = TopEdge{
			From:         r.FromNode,
			To:           r.ToNode,
			RelationType: r.RelationType,
			Confidence:   r.Confidence,
		}
	}
	return edges, int(totalCount), nil
}
```

This requires `MemoryService` to expose the `*gorm.DB`. Add a `DB()` method to `memory/memory.go`:

```go
// DB returns the underlying *gorm.DB for advanced queries.
func (srv *MemoryService) DB() *gorm.DB {
	return srv.s.db
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestQuery" -v 2>&1
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd E:/DEV/mcpdocs && rtk git add cmd/report/report.go cmd/report/report_test.go memory/memory.go && rtk git commit -m "feat(report): add queryTopNodes and queryEdges; expose DB() on MemoryService"
```

---

### Task 5: Add `GenerateReport` orchestrator and `main.go`

**Files:**
- Modify: `cmd/report/report.go`
- Create: `cmd/report/main.go`
- Modify: `cmd/report/report_test.go`

- [ ] **Step 1: Write failing test for `GenerateReport`**

Append to `cmd/report/report_test.go`:

```go
func TestGenerateReport(t *testing.T) {
	svc := setupTestDB(t)
	out, err := GenerateReport(svc, ReportConfig{
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
	db, _ := memory.OpenDB("")
	svc := memory.NewMemoryService(db)
	out, err := GenerateReport(svc, ReportConfig{Repo: "org/repo", MaxNodes: 20, MaxEdges: 40})
	if err != nil {
		t.Fatalf("GenerateReport empty: %v", err)
	}
	if !strings.Contains(out, "## DocScout Graph Analysis") {
		t.Error("missing header even on empty graph")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -run "TestGenerateReport" -v 2>&1
```
Expected: compile error — `GenerateReport`, `ReportConfig` undefined.

- [ ] **Step 3: Implement `ReportConfig` and `GenerateReport` in `report.go`**

Append to `cmd/report/report.go`:

```go
// ReportConfig holds the parameters for GenerateReport.
type ReportConfig struct {
	Repo     string
	Elapsed  int
	MaxNodes int
	MaxEdges int
}

// GenerateReport builds the full Markdown+Mermaid report string.
func GenerateReport(svc *memory.MemoryService, cfg ReportConfig) (string, error) {
	typeCounts, err := svc.EntityTypeCounts()
	if err != nil {
		return "", fmt.Errorf("entity type counts: %w", err)
	}

	totalEntities, err := svc.EntityCount()
	if err != nil {
		return "", fmt.Errorf("entity count: %w", err)
	}

	totalRelations, err := countRelations(svc)
	if err != nil {
		return "", fmt.Errorf("relation count: %w", err)
	}

	nodes, totalNodes, err := queryTopNodes(svc, cfg.MaxNodes)
	if err != nil {
		return "", fmt.Errorf("top nodes: %w", err)
	}

	edges, totalEdges, err := queryEdges(svc, nodes, cfg.MaxEdges)
	if err != nil {
		return "", fmt.Errorf("edges: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## DocScout Graph Analysis\n\n")
	if cfg.Repo != "" {
		fmt.Fprintf(&b, "Scan: `%s` · %d entities · %d relations · %ds\n\n",
			cfg.Repo, totalEntities, totalRelations, cfg.Elapsed)
	} else {
		fmt.Fprintf(&b, "%d entities · %d relations · %ds\n\n",
			totalEntities, totalRelations, cfg.Elapsed)
	}

	if pie := buildPie(typeCounts); pie != "" {
		b.WriteString("### Entity Distribution\n\n")
		b.WriteString(pie)
		b.WriteString("\n\n")
	}

	if chart := buildFlowchart(nodes, edges, cfg.MaxNodes, cfg.MaxEdges, totalNodes, totalEdges); chart != "" {
		b.WriteString("### Service Topology\n\n")
		b.WriteString(chart)
		b.WriteString("\n")
	}

	return b.String(), nil
}

// countRelations returns the total number of relations in the DB.
func countRelations(svc *memory.MemoryService) (int64, error) {
	var count int64
	err := svc.DB().Table("db_relations").Count(&count).Error
	return count, err
}
```

- [ ] **Step 4: Create `cmd/report/main.go`**

```go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/cmd/report"
)

func main() {
	dbPath := flag.String("db", "", "path to SQLite database (required)")
	maxNodes := flag.Int("max-nodes", 20, "max nodes in topology flowchart")
	maxEdges := flag.Int("max-edges", 40, "max edges in topology flowchart")
	repo := flag.String("repo", "", "org/repo label")
	elapsed := flag.Int("elapsed", 0, "scan duration in seconds")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required")
		os.Exit(1)
	}

	db, err := memory.OpenDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open db: %v\n", err)
		os.Exit(1)
	}

	svc := memory.NewMemoryService(db)
	out, err := report.GenerateReport(svc, report.ReportConfig{
		Repo:     *repo,
		Elapsed:  *elapsed,
		MaxNodes: *maxNodes,
		MaxEdges: *maxEdges,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: generate report: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(out)
}
```

Note: `main.go` uses package `main` but imports `report` as a package — this requires moving the report logic to a sub-package OR keeping both files in `package main`. Since `report_test.go` is in `package report` (not `package report_test`), use **`package report`** for `report.go` and `report_test.go`, and **`package main`** only for `main.go`. The import path becomes `github.com/doc-scout/mcp-server/cmd/report`.

Adjust `main.go` import to `report "github.com/doc-scout/mcp-server/cmd/report"` and call `report.GenerateReport(...)` and `report.ReportConfig{...}`.

- [ ] **Step 5: Run all tests**

```bash
cd E:/DEV/mcpdocs && go test ./cmd/report/... -v 2>&1
```
Expected: all PASS.

- [ ] **Step 6: Verify it compiles**

```bash
cd E:/DEV/mcpdocs && go build ./cmd/report/... 2>&1
```
Expected: no output (success).

- [ ] **Step 7: Commit**

```bash
cd E:/DEV/mcpdocs && rtk git add cmd/report/ memory/memory.go && rtk git commit -m "feat(report): add GenerateReport orchestrator and main entrypoint"
```

---

### Task 6: Update `action.yml` and `bin/run-scan.sh`

**Files:**
- Modify: `action.yml`
- Modify: `bin/run-scan.sh`

- [ ] **Step 1: Add `max_nodes` and `max_edges` inputs + `setup-go` step to `action.yml`**

In `action.yml`, add to the `inputs:` block (after `entity_types`):

```yaml
  max_nodes:
    description: Maximum number of nodes in the service topology diagram
    required: false
    default: '20'
  max_edges:
    description: Maximum number of edges in the service topology diagram
    required: false
    default: '40'
```

Add a new step before `Install DocScout` (after `actions/checkout@v4` in the example workflow — but in `action.yml` this is the first step):

```yaml
    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version-file: '${{ github.action_path }}/go.mod'
        cache: false
```

Update the `Run scan` step env block to add:

```yaml
        MAX_NODES: ${{ inputs.max_nodes }}
        MAX_EDGES: ${{ inputs.max_edges }}
```

- [ ] **Step 2: Replace summary and PR comment generation in `bin/run-scan.sh`**

Remove the two blocks labelled `# ── Write GitHub Step Summary` and `# ── Optional PR comment` (lines 120–166). Replace with:

```bash
# ── Generate rich Mermaid report ─────────────────────────────────────────────
REPORT=""
if command -v go &>/dev/null; then
  REPORT="$(go run "${GITHUB_ACTION_PATH}/cmd/report" \
    --db "$TMPDB" \
    --max-nodes "${MAX_NODES:-20}" \
    --max-edges "${MAX_EDGES:-40}" \
    --repo "$GITHUB_REPOSITORY" \
    --elapsed "$ELAPSED" 2>/dev/null)" || REPORT=""
fi

# ── Write GitHub Step Summary ─────────────────────────────────────────────────
if [ -n "$REPORT" ]; then
  echo "$REPORT" >> "${GITHUB_STEP_SUMMARY}"
else
  {
    echo "## DocScout Graph Analysis"
    echo ""
    echo "| Metric | Count |"
    echo "|--------|-------|"
    echo "| Entities | ${ENTITY_COUNT} |"
    echo "| Relations | ${RELATION_COUNT} |"
    echo ""
    echo "Scan completed in ${ELAPSED}s for \`${GITHUB_REPOSITORY}\`"
  } >> "${GITHUB_STEP_SUMMARY}"
fi

# ── Optional PR comment ───────────────────────────────────────────────────────
if [ "${COMMENT_ON_PR}" = "true" ] && [ -n "${PR_NUMBER}" ]; then
  if [ -z "$REPORT" ]; then
    REPORT="## DocScout Graph Analysis

| Metric | Count |
|--------|-------|
| Entities | ${ENTITY_COUNT} |
| Relations | ${RELATION_COUNT} |

Scan completed in ${ELAPSED}s for \`${GITHUB_REPOSITORY}\`"
  fi

  if command -v gh &>/dev/null; then
    echo "Posting PR comment on PR #${PR_NUMBER}..." >&2
    gh pr comment "${PR_NUMBER}" \
      --body "${REPORT}" \
      --edit-last 2>/dev/null \
      || gh pr comment "${PR_NUMBER}" --body "${REPORT}"
  else
    echo "Warning: 'gh' CLI not found — skipping PR comment." >&2
  fi
fi
```

Also remove the `ENTITY_TYPE_SECTION` block (lines 108–118) and the `entity_types` section from the summary — the pie chart supersedes it.

- [ ] **Step 3: Verify the shell script is valid**

```bash
bash -n E:/DEV/mcpdocs/bin/run-scan.sh 2>&1
```
Expected: no output (valid syntax).

- [ ] **Step 4: Commit**

```bash
cd E:/DEV/mcpdocs && rtk git add action.yml bin/run-scan.sh && rtk git commit -m "feat(action): add max_nodes/max_edges inputs; use go run ./cmd/report for rich Mermaid output"
```

---

### Task 7: Final verification and PR

- [ ] **Step 1: Run the full test suite**

```bash
cd E:/DEV/mcpdocs && go test ./... 2>&1 | tail -20
```
Expected: all packages PASS (or skip). No new failures.

- [ ] **Step 2: Run `go vet`**

```bash
cd E:/DEV/mcpdocs && go vet ./... 2>&1
```
Expected: no output.

- [ ] **Step 3: Run `mise run format` (gci import sort)**

```bash
cd E:/DEV/mcpdocs && mise run format 2>&1
```

- [ ] **Step 4: Commit any formatting changes**

```bash
cd E:/DEV/mcpdocs && rtk git diff --stat && rtk git add -p && rtk git commit -m "style: apply gci formatting to cmd/report" 2>/dev/null || true
```

- [ ] **Step 5: Push branch and open PR**

```bash
cd E:/DEV/mcpdocs && rtk git push -u origin feat/mermaid-graph-report
env -u GITHUB_TOKEN gh pr create \
  --base main \
  --head feat/mermaid-graph-report \
  --title "feat: rich Mermaid graph report in GitHub Actions" \
  --body "## Summary

- Adds \`cmd/report\` Go package that generates a Markdown+Mermaid report from the DocScout SQLite database
- Replaces the simple entity/relation counter table in the Step Summary and PR comment with:
  - A **pie chart** showing entity distribution by type
  - A **service topology flowchart** (top N nodes by connectivity, solid/dashed arrows for authoritative/inferred relations, shapes per entity type)
- New \`action.yml\` inputs: \`max_nodes\` (default 20) and \`max_edges\` (default 40)
- Graceful fallback to plain table if \`go\` is unavailable or report generation fails

## Test plan
- [ ] All unit tests pass (\`go test ./cmd/report/...\`)
- [ ] \`action.yml\` step summary renders Mermaid diagram in a workflow run
- [ ] PR comment updated with Mermaid diagram when \`comment_on_pr: true\`
- [ ] Truncation warning appears when node/edge count exceeds cap
- [ ] Inferred edges render as dashed arrows (\`-.->\`)
- [ ] Fallback plain table appears when \`go\` binary is not available"
```
