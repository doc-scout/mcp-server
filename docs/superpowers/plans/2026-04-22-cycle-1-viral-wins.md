# Cycle 1 — Viral Wins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the three highest-ROI visibility features identified in the ecosystem analysis: a corrected benchmark badge (Item 27), an interactive graph visualization export tool (Item 25), and a 60-second onboarding script (Item 28).

**Architecture:** Item 27 is a doc-only change. Item 25 adds `memory/export.go` (standalone pure function, no interface change) and `tools/export_graph.go` (MCP tool that calls `ReadGraph()` then renders). Item 28 adds `bin/docscout-init.sh` (curl-installable shell script).

**Tech Stack:** Go 1.26, `text/template`, `encoding/json`, vanilla JS Canvas 2D (self-contained HTML, no CDN), Bash/sh for the init script.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Modify | `README.md` | Fix badge from 93% → 99% |
| Modify | `docs/index.md` | Add matching token-savings badge |
| Create | `memory/export.go` | `ExportGraph(kg KnowledgeGraph, format, title string) ([]byte, error)` |
| Create | `memory/export_test.go` | Unit tests for HTML + JSON export |
| Create | `tools/export_graph.go` | `export_graph` MCP tool handler |
| Modify | `tools/tools.go` | Register `export_graph` tool |
| Create | `bin/docscout-init.sh` | One-command onboarding script |
| Modify | `docs/contributing.md` | Add init script to quick-start |
| Modify | `ROADMAP.md` | Mark items 25, 27, 28 complete |

---

## Task 1: Fix Benchmark Badge (Item 27)

**Files:**
- Modify: `README.md:14`
- Modify: `docs/index.md` (add badge line)

The `benchmark/RESULTS.md` shows 99.0% average token savings but the README badge still says 93%. Fix it.

- [ ] **Step 1: Update badge in README.md**

Find line 14 in `README.md`:
```
[![Token Savings](https://img.shields.io/badge/token--savings-93%25-brightgreen)](benchmark/RESULTS.md)
```
Replace with:
```
[![Token Savings](https://img.shields.io/badge/token--savings-99%25-brightgreen)](benchmark/RESULTS.md)
```

- [ ] **Step 2: Add badges to docs/index.md**

In `docs/index.md`, find the three existing badges block:
```markdown
[![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://github.com/doc-scout/mcp-server/blob/main/LICENSE)
[![MCP](https://img.shields.io/badge/MCP-compatible-blueviolet)](https://modelcontextprotocol.io)
```
Replace with (add the two new badges):
```markdown
[![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://github.com/doc-scout/mcp-server/blob/main/LICENSE)
[![MCP](https://img.shields.io/badge/MCP-compatible-blueviolet)](https://modelcontextprotocol.io)
[![Token Savings](https://img.shields.io/badge/token--savings-99%25-brightgreen)](https://github.com/doc-scout/mcp-server/blob/main/benchmark/RESULTS.md)
[![Graph Accuracy F1](https://img.shields.io/badge/graph--accuracy-F1%201.00-blue)](https://github.com/doc-scout/mcp-server/blob/main/benchmark/RESULTS.md)
```

- [ ] **Step 3: Commit**

```bash
git add README.md docs/index.md
git -c commit.gpgsign=false commit -m "docs: fix token savings badge 93%→99%, add badges to docs site"
```

---

## Task 2: Implement memory/export.go (Item 25 — backend)

**Files:**
- Create: `memory/export.go`

This file provides a standalone pure function `ExportGraph` — no interface changes, no DB access, operates on the already-loaded `KnowledgeGraph` struct. Supports `"html"` and `"json"` formats.

- [ ] **Step 1: Write the failing test first**

Create `memory/export_test.go`:

```go
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
```

- [ ] **Step 2: Run test — verify it fails**

```bash
go test ./memory/... -run TestExportGraph -v
```

Expected: `FAIL — undefined: memory.ExportGraph`

- [ ] **Step 3: Create memory/export.go**

```go
// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
)

// ExportGraph serializes kg into the requested format.
// format must be "html" or "json".
// title is used as the graph/document title.
func ExportGraph(kg KnowledgeGraph, format, title string) ([]byte, error) {
	switch format {
	case "json":
		return exportJSON(kg, title)
	case "html":
		return exportHTML(kg, title)
	default:
		return nil, fmt.Errorf("unsupported export format %q: use 'html' or 'json'", format)
	}
}

// --- JSON export ---

type exportNode struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Type         string   `json:"type"`
	Observations []string `json:"observations"`
}

type exportEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

type exportJSON struct {
	Title string       `json:"title"`
	Nodes []exportNode `json:"nodes"`
	Edges []exportEdge `json:"edges"`
}

func exportJSON(kg KnowledgeGraph, title string) ([]byte, error) {
	out := exportJSON{Title: title}
	for _, e := range kg.Entities {
		out.Nodes = append(out.Nodes, exportNode{
			ID:           e.Name,
			Label:        e.Name,
			Type:         e.EntityType,
			Observations: e.Observations,
		})
	}
	for _, r := range kg.Relations {
		out.Edges = append(out.Edges, exportEdge{From: r.From, To: r.To, Label: r.RelationType})
	}
	return json.MarshalIndent(out, "", "  ")
}

// --- HTML export ---

type htmlData struct {
	Title         string
	EntityCount   int
	RelationCount int
	NodesJSON     string
	EdgesJSON     string
}

func exportHTML(kg KnowledgeGraph, title string) ([]byte, error) {
	type jsNode struct {
		ID    string   `json:"id"`
		Label string   `json:"label"`
		Type  string   `json:"type"`
		Obs   []string `json:"obs"`
	}
	type jsEdge struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Label string `json:"label"`
	}

	nodes := make([]jsNode, 0, len(kg.Entities))
	for _, e := range kg.Entities {
		obs := e.Observations
		if obs == nil {
			obs = []string{}
		}
		nodes = append(nodes, jsNode{ID: e.Name, Label: e.Name, Type: e.EntityType, Obs: obs})
	}
	edges := make([]jsEdge, 0, len(kg.Relations))
	for _, r := range kg.Relations {
		edges = append(edges, jsEdge{From: r.From, To: r.To, Label: r.RelationType})
	}

	nodesJSON, err := json.Marshal(nodes)
	if err != nil {
		return nil, err
	}
	edgesJSON, err := json.Marshal(edges)
	if err != nil {
		return nil, err
	}

	data := htmlData{
		Title:         title,
		EntityCount:   len(kg.Entities),
		RelationCount: len(kg.Relations),
		NodesJSON:     string(nodesJSON),
		EdgesJSON:     string(edgesJSON),
	}

	tmpl, err := template.New("graph").Parse(graphHTMLTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// graphHTMLTemplate is a self-contained HTML page with a vanilla-JS force-directed graph.
// No external dependencies — works fully offline.
const graphHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>DocScout — {{.Title}}</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0d1117;color:#e6edf3;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",monospace;overflow:hidden}
canvas{display:block}
#hud{position:fixed;top:12px;left:12px;background:rgba(22,27,34,.92);border:1px solid #30363d;border-radius:8px;padding:12px 16px;font-size:13px;max-width:300px;line-height:1.6}
#hud strong{color:#58a6ff;font-size:14px}
#hud small{color:#8b949e}
#tip{position:fixed;pointer-events:none;background:#161b22;border:1px solid #58a6ff;border-radius:6px;padding:8px 12px;font-size:12px;max-width:280px;display:none;line-height:1.5;z-index:10}
#tip .tname{font-weight:700;color:#58a6ff;margin-bottom:4px}
#tip .ttype{color:#8b949e;font-size:11px;margin-bottom:6px}
#tip .tobs{color:#adbac7}
</style>
</head>
<body>
<canvas id="c"></canvas>
<div id="hud">
  <strong>{{.Title}}</strong><br>
  {{.EntityCount}} entities &middot; {{.RelationCount}} relations<br>
  <small>Scroll&nbsp;zoom &middot; Drag&nbsp;pan &middot; Hover&nbsp;details</small>
</div>
<div id="tip"><div class="tname" id="tn"></div><div class="ttype" id="tt"></div><div class="tobs" id="to"></div></div>
<script>
const NODES=` + "`" + `{{.NodesJSON}}` + "`" + `;
const EDGES=` + "`" + `{{.EdgesJSON}}` + "`" + `;
const nodes=JSON.parse(NODES),edges=JSON.parse(EDGES);
const canvas=document.getElementById('c'),ctx=canvas.getContext('2d');
const tip=document.getElementById('tip');
let W=0,H=0,ox=0,oy=0,scale=1,dragging=false,dragStart={x:0,y:0},panStart={x:0,y:0};

const palette=['#58a6ff','#3fb950','#ff7b72','#ffa657','#d2a8ff','#79c0ff','#56d364','#f0883e'];
const typeColor={};let ci=0;
function colorFor(t){if(!typeColor[t])typeColor[t]=palette[ci++%palette.length];return typeColor[t];}

function resize(){W=window.innerWidth;H=window.innerHeight;canvas.width=W;canvas.height=H;}
window.addEventListener('resize',()=>{resize();});
resize();

// Init positions
nodes.forEach((n,i)=>{
  const angle=(i/nodes.length)*2*Math.PI;
  const r=Math.min(W,H)*0.3;
  n.x=W/2+r*Math.cos(angle)+( Math.random()-.5)*60;
  n.y=H/2+r*Math.sin(angle)+(Math.random()-.5)*60;
  n.vx=0;n.vy=0;n.pinned=false;
});

const idxOf=Object.fromEntries(nodes.map((n,i)=>[n.id,i]));

let iteration=0;
function tick(){
  if(iteration++>600)return;
  const k=Math.sqrt((W*H)/Math.max(nodes.length,1))*0.5;
  nodes.forEach(n=>{n.fx=0;n.fy=0;});
  // repulsion
  for(let i=0;i<nodes.length;i++)for(let j=i+1;j<nodes.length;j++){
    const a=nodes[i],b=nodes[j];
    const dx=a.x-b.x,dy=a.y-b.y;
    const d=Math.max(Math.sqrt(dx*dx+dy*dy),1);
    const f=k*k/d*1.5;
    const fx=dx/d*f,fy=dy/d*f;
    a.fx+=fx;a.fy+=fy;b.fx-=fx;b.fy-=fy;
  }
  // attraction
  edges.forEach(e=>{
    const a=nodes[idxOf[e.from]],b=nodes[idxOf[e.to]];
    if(!a||!b)return;
    const dx=b.x-a.x,dy=b.y-a.y;
    const d=Math.max(Math.sqrt(dx*dx+dy*dy),1);
    const f=d*d/k*0.05;
    const fx=dx/d*f,fy=dy/d*f;
    a.fx+=fx;a.fy+=fy;b.fx-=fx;b.fy-=fy;
  });
  // gravity
  nodes.forEach(n=>{n.fx+=(W/2-n.x)*0.008;n.fy+=(H/2-n.y)*0.008;});
  // integrate
  nodes.forEach(n=>{
    if(n.pinned)return;
    n.vx=(n.vx+n.fx)*0.82;n.vy=(n.vy+n.fy)*0.82;
    n.x+=n.vx;n.y+=n.vy;
  });
}

function toScreen(wx,wy){return{x:(wx+ox)*scale+W/2,y:(wy+oy)*scale+H/2};}
function toWorld(sx,sy){return{x:(sx-W/2)/scale-ox,y:(sy-H/2)/scale-oy};}

const R=14;
function draw(){
  ctx.clearRect(0,0,W,H);
  ctx.save();
  ctx.translate(W/2,H/2);ctx.scale(scale,scale);ctx.translate(ox,oy);
  // edges
  edges.forEach(e=>{
    const a=nodes[idxOf[e.from]],b=nodes[idxOf[e.to]];
    if(!a||!b)return;
    ctx.beginPath();ctx.moveTo(a.x,a.y);ctx.lineTo(b.x,b.y);
    ctx.strokeStyle='#30363d';ctx.lineWidth=1.2/scale;ctx.stroke();
    // label
    const mx=(a.x+b.x)/2,my=(a.y+b.y)/2;
    ctx.font=`${10/scale}px monospace`;ctx.fillStyle='#6e7681';ctx.textAlign='center';
    ctx.fillText(e.label,mx,my-4/scale);
    // arrow
    const angle=Math.atan2(b.y-a.y,b.x-a.x);
    const ax=b.x-Math.cos(angle)*(R+2)/scale,ay=b.y-Math.sin(angle)*(R+2)/scale;
    const as=6/scale;
    ctx.beginPath();ctx.moveTo(ax,ay);
    ctx.lineTo(ax-Math.cos(angle-0.4)*as,ay-Math.sin(angle-0.4)*as);
    ctx.lineTo(ax-Math.cos(angle+0.4)*as,ay-Math.sin(angle+0.4)*as);
    ctx.closePath();ctx.fillStyle='#30363d';ctx.fill();
  });
  // nodes
  nodes.forEach(n=>{
    const c=colorFor(n.type);
    ctx.beginPath();ctx.arc(n.x,n.y,R/scale,0,Math.PI*2);
    ctx.fillStyle=c+'22';ctx.fill();
    ctx.strokeStyle=c;ctx.lineWidth=1.5/scale;ctx.stroke();
    ctx.font=`bold ${11/scale}px -apple-system,monospace`;
    ctx.fillStyle='#e6edf3';ctx.textAlign='center';
    ctx.fillText(n.label,n.x,n.y+R*1.8/scale);
  });
  ctx.restore();
}

let hovNode=null;
canvas.addEventListener('mousemove',e=>{
  const {x,y}=toWorld(e.clientX,e.clientY);
  hovNode=null;
  for(const n of nodes){
    const d=Math.hypot(n.x-x,n.y-y);
    if(d<R/scale+4){hovNode=n;break;}
  }
  if(hovNode){
    document.getElementById('tn').textContent=hovNode.label;
    document.getElementById('tt').textContent=hovNode.type;
    document.getElementById('to').textContent=(hovNode.obs||[]).join('\n')||'(no observations)';
    tip.style.display='block';
    tip.style.left=(e.clientX+14)+'px';
    tip.style.top=(e.clientY-8)+'px';
  }else{tip.style.display='none';}
  if(dragging){ox+=( e.clientX-dragStart.x)/scale;oy+=(e.clientY-dragStart.y)/scale;dragStart={x:e.clientX,y:e.clientY};}
});
canvas.addEventListener('mouseleave',()=>{tip.style.display='none';});
canvas.addEventListener('mousedown',e=>{dragging=true;dragStart={x:e.clientX,y:e.clientY};});
canvas.addEventListener('mouseup',()=>{dragging=false;});
canvas.addEventListener('wheel',e=>{
  e.preventDefault();
  const factor=e.deltaY<0?1.1:0.91;
  scale=Math.max(0.1,Math.min(10,scale*factor));
},{passive:false});

function loop(){tick();draw();requestAnimationFrame(loop);}
loop();
</script>
</body>
</html>`
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./memory/... -run TestExportGraph -v
```

Expected:
```
--- PASS: TestExportGraphJSON
--- PASS: TestExportGraphHTML
--- PASS: TestExportGraphUnknownFormat
```

- [ ] **Step 5: Commit**

```bash
git add memory/export.go memory/export_test.go
git -c commit.gpgsign=false commit -m "feat: add ExportGraph function (html + json formats)"
```

---

## Task 3: Implement tools/export_graph.go (Item 25 — MCP tool)

**Files:**
- Create: `tools/export_graph.go`
- Modify: `tools/tools.go` (register the tool)

The handler calls `g.ReadGraph()`, then `memory.ExportGraph(kg, format, title)`, optionally writes to `output_path`.

- [ ] **Step 1: Create tools/export_graph.go**

```go
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
	Format     string `json:"format,omitempty"      jsonschema:"Export format: 'html' (interactive force graph, offline) or 'json' (nodes+edges). Defaults to 'html'."`
	Title      string `json:"title,omitempty"       jsonschema:"Title shown in the exported artifact (e.g. org name). Defaults to 'Knowledge Graph'."`
	OutputPath string `json:"output_path,omitempty" jsonschema:"Absolute path where the file will be written (e.g. /tmp/graph.html). If omitted the content is returned inline (base64 for html, plain for json)."`
}

// ExportGraphResult is the structured output of the export_graph tool.
type ExportGraphResult struct {
	Format      string `json:"format"`
	EntityCount int    `json:"entity_count"`
	EdgeCount   int    `json:"edge_count"`
	OutputPath  string `json:"output_path,omitempty"`
	Content     string `json:"content,omitempty"`
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
			Format:      format,
			EntityCount: len(kg.Entities),
			EdgeCount:   len(kg.Relations),
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
```

- [ ] **Step 2: Register the tool in tools/tools.go**

In `tools/tools.go`, find the block that registers `get_integration_map` (near the end of the `graph != nil` block). After it, add:

```go
		mcp.AddTool(s, &mcp.Tool{
			Name: "export_graph",
			Description: "Exports the entire knowledge graph as an interactive HTML visualization or JSON. " +
				"The HTML format produces a self-contained, offline-capable force-directed graph — " +
				"no internet connection required. Use output_path to write the file directly to disk " +
				"(e.g. output_path='/tmp/graph.html'), or omit it to receive the content inline. " +
				"Open the resulting HTML file in any browser to explore entities, relations, and observations.",
		}, withMetrics("export_graph", metrics, withRecovery("export_graph", exportGraphHandler(graph))))
```

- [ ] **Step 3: Run full test suite**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all packages PASS. If any fail, fix before continuing.

- [ ] **Step 4: Build binary to confirm compilation**

```bash
go build -o /tmp/docscout-test . && echo "BUILD OK"
```

Expected: `BUILD OK`

- [ ] **Step 5: Commit**

```bash
git add tools/export_graph.go tools/tools.go
git -c commit.gpgsign=false commit -m "feat: add export_graph MCP tool (html + json graph visualization)"
```

---

## Task 4: Onboarding Script (Item 28)

**Files:**
- Create: `bin/docscout-init.sh`

A curl-installable shell script that downloads the latest binary (or uses `go run` as fallback), creates `.env.local` with sane defaults, and prints the Claude Desktop config snippet.

- [ ] **Step 1: Create bin/docscout-init.sh**

```bash
#!/usr/bin/env sh
# DocScout-MCP — one-command setup
# Usage: curl -fsSL https://raw.githubusercontent.com/doc-scout/mcp-server/main/bin/docscout-init.sh | sh
set -e

REPO="doc-scout/mcp-server"
BINARY="docscout-mcp"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

echo ""
echo "  DocScout-MCP — quick setup"
echo "  ────────────────────────────────"
echo ""

# ── Step 1: collect inputs ────────────────────────────────────────────────────
printf "GitHub PAT (fine-grained, read-only Contents+Metadata): "
read -r GITHUB_TOKEN

printf "GitHub org or username to scan: "
read -r GITHUB_ORG

if [ -z "$GITHUB_TOKEN" ] || [ -z "$GITHUB_ORG" ]; then
  echo "Error: GITHUB_TOKEN and GITHUB_ORG are required." >&2
  exit 1
fi

# ── Step 2: detect OS/arch ────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
esac

# ── Step 3: download latest binary ───────────────────────────────────────────
LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Warning: could not determine latest release; falling back to go run." >&2
  USE_GORUN=1
fi

if [ -z "$USE_GORUN" ]; then
  ASSET="${BINARY}-${OS}-${ARCH}"
  URL="https://github.com/$REPO/releases/download/$LATEST/${ASSET}"
  mkdir -p "$INSTALL_DIR"
  echo "Downloading $BINARY $LATEST ($OS/$ARCH)…"
  if curl -fsSL "$URL" -o "$INSTALL_DIR/$BINARY"; then
    chmod +x "$INSTALL_DIR/$BINARY"
    BINARY_PATH="$INSTALL_DIR/$BINARY"
    echo "  ✓ installed to $BINARY_PATH"
  else
    echo "  Binary not found for $OS/$ARCH — falling back to go run." >&2
    USE_GORUN=1
  fi
fi

if [ -n "$USE_GORUN" ]; then
  BINARY_PATH="go run github.com/doc-scout/mcp-server@latest"
fi

# ── Step 4: write .env.local ──────────────────────────────────────────────────
ENV_FILE="./.env.local"
cat > "$ENV_FILE" <<EOF
GITHUB_TOKEN=$GITHUB_TOKEN
GITHUB_ORG=$GITHUB_ORG
SCAN_INTERVAL=30m
DATABASE_URL=sqlite://docscout.db
EOF
echo "  ✓ wrote $ENV_FILE"

# ── Step 5: print Claude Desktop config ──────────────────────────────────────
echo ""
echo "  Add this to your claude_desktop_config.json (mcpServers block):"
echo ""
cat <<EOF
{
  "docscout": {
    "command": "$BINARY_PATH",
    "env": {
      "GITHUB_TOKEN": "$GITHUB_TOKEN",
      "GITHUB_ORG": "$GITHUB_ORG",
      "SCAN_INTERVAL": "30m",
      "DATABASE_URL": "sqlite://docscout.db"
    }
  }
}
EOF
echo ""
echo "  ✓ Done! Start the server:"
echo ""
if [ -n "$USE_GORUN" ]; then
  echo "    go run github.com/doc-scout/mcp-server@latest"
else
  echo "    $BINARY_PATH"
fi
echo ""
```

- [ ] **Step 2: Make executable**

```bash
chmod +x bin/docscout-init.sh
```

- [ ] **Step 3: Smoke-test the script logic (dry run)**

```bash
# Verify the script is valid sh syntax
sh -n bin/docscout-init.sh && echo "SYNTAX OK"
```

Expected: `SYNTAX OK`

- [ ] **Step 4: Add install instructions to docs/contributing.md**

In `docs/contributing.md`, find the Quick Start section at the top. After the `git clone / go mod download` block, add:

```markdown
## Try DocScout in 60 Seconds

No Git clone needed:

```sh
curl -fsSL https://raw.githubusercontent.com/doc-scout/mcp-server/main/bin/docscout-init.sh | sh
```

The script downloads the latest binary, writes `.env.local`, and prints the Claude Desktop config snippet.
```

- [ ] **Step 5: Commit**

```bash
git add bin/docscout-init.sh docs/contributing.md
git -c commit.gpgsign=false commit -m "feat: add docscout-init.sh — one-command 60s onboarding"
```

---

## Task 5: Update ROADMAP.md + finalize PR

**Files:**
- Modify: `ROADMAP.md`

- [ ] **Step 1: Mark items 25, 27, 28 as complete in ROADMAP.md**

In `ROADMAP.md`, move items 25, 27, and 28 from the `## Future Work` section to `## Completed`. Add `✅` and implementation notes. Use the same format as existing completed items.

For item 27, add under Completed:
```markdown
### 27. Benchmark Narrative — "The Number" ✅
- **Implemented**: `benchmark/RESULTS.md` — 99.0% average token savings across 12 canonical questions, F1 1.00 parser accuracy across all 10 parsers. Badge added to README and docs site.
```

For item 25, add under Completed:
```markdown
### 25. Graph Visualization Export ✅
- **Implemented**: `memory/export.go` — `ExportGraph(kg, format, title)` renders `html` (self-contained force-directed graph, vanilla JS Canvas, zero dependencies) and `json` (nodes+edges) formats.
- `tools/export_graph.go` — `export_graph` MCP tool: accepts `format`, `title`, and optional `output_path`. Writes to disk or returns content inline.
```

For item 28, add under Completed:
```markdown
### 28. Onboarding in 60 Seconds ✅
- **Implemented**: `bin/docscout-init.sh` — curl-installable shell script. Downloads latest binary for the detected OS/arch (falls back to `go run`), writes `.env.local`, and prints the Claude Desktop config snippet. No manual config required for the "try it" path.
```

- [ ] **Step 2: Run full test suite one final time**

```bash
go test ./... 2>&1 | tail -10
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add ROADMAP.md
git -c commit.gpgsign=false commit -m "docs: mark items 25, 27, 28 complete in roadmap"
```

- [ ] **Step 4: Push branch and open PR**

```bash
git push -u origin feat/cycle-1-viral-wins
gh pr create \
  --title "feat: Cycle 1 — graph export, benchmark badge, 60s onboarding" \
  --body "$(cat <<'EOF'
## Summary

- **Item 27 (The Number):** Fixed token-savings badge from 93% → 99% (matches benchmark/RESULTS.md). Added badges to docs site.
- **Item 25 (Graph Visualization):** New `export_graph` MCP tool — exports the knowledge graph as a self-contained interactive HTML (force-directed, vanilla JS, zero deps) or JSON. No interface changes.
- **Item 28 (Onboarding 60s):** New `bin/docscout-init.sh` — curl-installable, downloads binary, writes `.env.local`, prints Claude Desktop config. Targets zero manual config for the "try it" path.

## Test plan

- [ ] `go test ./...` passes
- [ ] `go build .` succeeds
- [ ] `export_graph` with `format=html` produces a valid HTML file that renders in browser
- [ ] `export_graph` with `format=json` produces valid JSON with `nodes` + `edges` arrays
- [ ] `export_graph` with `output_path=/tmp/graph.html` writes the file to disk
- [ ] `bin/docscout-init.sh` passes `sh -n` syntax check
- [ ] README badges display correctly on GitHub
EOF
)"
```

---

## Self-Review Notes

- **Spec coverage:** Items 25 (export_graph tool ✓), 27 (badge + RESULTS.md ✓), 28 (init script ✓). Item 20 was already implemented — only the badge needed updating.
- **No placeholders:** All steps contain actual code.
- **Type consistency:** `ExportGraphArgs`, `ExportGraphResult` defined in `export_graph.go` and not referenced elsewhere. `memory.ExportGraph` signature used consistently.
- **Interface:** No `GraphStore` interface changes — `ReadGraph()` already exists. `memory.ExportGraph` is a standalone function, no `GraphAuditLogger` pass-through needed.
- **STDIO safety:** `export_graph` writes to `os.Stderr` via `slog` (panic recovery) and to a user-supplied `output_path` only — never to `stdout`.
