// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
)

// ExportGraph serializes kg into the requested format.
// format must be "html" or "json".
// title is used as the graph/document title.
func ExportGraph(kg KnowledgeGraph, format, title string) ([]byte, error) {
	switch format {
	case "json":
		return exportGraphJSON(kg, title)
	case "html":
		return exportGraphHTML(kg, title)
	default:
		return nil, fmt.Errorf("unsupported export format %q: use 'html' or 'json'", format)
	}
}

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

type graphExportData struct {
	Title string       `json:"title"`
	Nodes []exportNode `json:"nodes"`
	Edges []exportEdge `json:"edges"`
}

func exportGraphJSON(kg KnowledgeGraph, title string) ([]byte, error) {
	out := graphExportData{
		Title: title,
		Nodes: make([]exportNode, 0, len(kg.Entities)),
		Edges: make([]exportEdge, 0, len(kg.Relations)),
	}
	for _, e := range kg.Entities {
		obs := e.Observations
		if obs == nil {
			obs = []string{}
		}
		out.Nodes = append(out.Nodes, exportNode{
			ID:           e.Name,
			Label:        e.Name,
			Type:         e.EntityType,
			Observations: obs,
		})
	}
	for _, r := range kg.Relations {
		out.Edges = append(out.Edges, exportEdge{From: r.From, To: r.To, Label: r.RelationType})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("exportGraphJSON: marshal: %w", err)
	}
	return b, nil
}

type htmlTemplateData struct {
	Title         string
	EntityCount   int
	RelationCount int
	NodesJSON     template.JS
	EdgesJSON     template.JS
}

var graphTmpl = template.Must(template.New("graph").Parse(graphHTMLTemplate))

func exportGraphHTML(kg KnowledgeGraph, title string) ([]byte, error) {
	nodes := make([]exportNode, 0, len(kg.Entities))
	for _, e := range kg.Entities {
		obs := e.Observations
		if obs == nil {
			obs = []string{}
		}
		nodes = append(nodes, exportNode{ID: e.Name, Label: e.Name, Type: e.EntityType, Observations: obs})
	}
	edges := make([]exportEdge, 0, len(kg.Relations))
	for _, r := range kg.Relations {
		edges = append(edges, exportEdge{From: r.From, To: r.To, Label: r.RelationType})
	}

	nodesJSON, err := json.Marshal(nodes)
	if err != nil {
		return nil, fmt.Errorf("exportGraphHTML: marshal nodes: %w", err)
	}
	edgesJSON, err := json.Marshal(edges)
	if err != nil {
		return nil, fmt.Errorf("exportGraphHTML: marshal edges: %w", err)
	}

	data := htmlTemplateData{
		Title:         title,
		EntityCount:   len(kg.Entities),
		RelationCount: len(kg.Relations),
		NodesJSON:     template.JS(nodesJSON),
		EdgesJSON:     template.JS(edgesJSON),
	}

	var buf bytes.Buffer
	if err := graphTmpl.Execute(&buf, data); err != nil {
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
let W=0,H=0,ox=0,oy=0,scale=1,dragging=false,dragStart={x:0,y:0};

const palette=['#58a6ff','#3fb950','#ff7b72','#ffa657','#d2a8ff','#79c0ff','#56d364','#f0883e'];
const typeColor={};let ci=0;
function colorFor(t){if(!typeColor[t])typeColor[t]=palette[ci++%palette.length];return typeColor[t];}

function resize(){W=window.innerWidth;H=window.innerHeight;canvas.width=W;canvas.height=H;}
window.addEventListener('resize',()=>{resize();});
resize();

nodes.forEach((n,i)=>{
  const angle=(i/nodes.length)*2*Math.PI;
  const r=Math.min(W,H)*0.3;
  n.x=W/2+r*Math.cos(angle)+(Math.random()-.5)*60;
  n.y=H/2+r*Math.sin(angle)+(Math.random()-.5)*60;
  n.vx=0;n.vy=0;
});

const idxOf=Object.fromEntries(nodes.map((n,i)=>[n.id,i]));

let iteration=0;
function tick(){
  if(iteration++>600)return;
  const k=Math.sqrt((W*H)/Math.max(nodes.length,1))*0.5;
  nodes.forEach(n=>{n.fx=0;n.fy=0;});
  for(let i=0;i<nodes.length;i++)for(let j=i+1;j<nodes.length;j++){
    const a=nodes[i],b=nodes[j];
    const dx=a.x-b.x,dy=a.y-b.y;
    const d=Math.max(Math.sqrt(dx*dx+dy*dy),1);
    const f=k*k/d*1.5;
    const fx=dx/d*f,fy=dy/d*f;
    a.fx+=fx;a.fy+=fy;b.fx-=fx;b.fy-=fy;
  }
  edges.forEach(e=>{
    const a=nodes[idxOf[e.from]],b=nodes[idxOf[e.to]];
    if(!a||!b)return;
    const dx=b.x-a.x,dy=b.y-a.y;
    const d=Math.max(Math.sqrt(dx*dx+dy*dy),1);
    const f=d*d/k*0.05;
    const fx=dx/d*f,fy=dy/d*f;
    a.fx+=fx;a.fy+=fy;b.fx-=fx;b.fy-=fy;
  });
  nodes.forEach(n=>{n.fx+=(W/2-n.x)*0.008;n.fy+=(H/2-n.y)*0.008;});
  nodes.forEach(n=>{
    n.vx=(n.vx+n.fx)*0.82;n.vy=(n.vy+n.fy)*0.82;
    n.x+=n.vx;n.y+=n.vy;
  });
}

function toWorld(sx,sy){return{x:(sx-W/2)/scale-ox,y:(sy-H/2)/scale-oy};}

const R=14;
function draw(){
  ctx.clearRect(0,0,W,H);
  ctx.save();
  ctx.translate(W/2,H/2);ctx.scale(scale,scale);ctx.translate(ox,oy);
  edges.forEach(e=>{
    const a=nodes[idxOf[e.from]],b=nodes[idxOf[e.to]];
    if(!a||!b)return;
    ctx.beginPath();ctx.moveTo(a.x,a.y);ctx.lineTo(b.x,b.y);
    ctx.strokeStyle='#30363d';ctx.lineWidth=1.2/scale;ctx.stroke();
    const mx=(a.x+b.x)/2,my=(a.y+b.y)/2;
    ctx.font=` + "`" + `${10/scale}px monospace` + "`" + `;ctx.fillStyle='#6e7681';ctx.textAlign='center';
    ctx.fillText(e.label,mx,my-4/scale);
    const angle=Math.atan2(b.y-a.y,b.x-a.x);
    const ax=b.x-Math.cos(angle)*(R+2)/scale,ay=b.y-Math.sin(angle)*(R+2)/scale;
    const as=6/scale;
    ctx.beginPath();ctx.moveTo(ax,ay);
    ctx.lineTo(ax-Math.cos(angle-0.4)*as,ay-Math.sin(angle-0.4)*as);
    ctx.lineTo(ax-Math.cos(angle+0.4)*as,ay-Math.sin(angle+0.4)*as);
    ctx.closePath();ctx.fillStyle='#30363d';ctx.fill();
  });
  nodes.forEach(n=>{
    const c=colorFor(n.type);
    ctx.beginPath();ctx.arc(n.x,n.y,R/scale,0,Math.PI*2);
    ctx.fillStyle=c+'22';ctx.fill();
    ctx.strokeStyle=c;ctx.lineWidth=1.5/scale;ctx.stroke();
    ctx.font=` + "`" + `bold ${11/scale}px -apple-system,monospace` + "`" + `;
    ctx.fillStyle='#e6edf3';ctx.textAlign='center';
    ctx.fillText(n.label,n.x,n.y+R*1.8/scale);
  });
  ctx.restore();
}

canvas.addEventListener('mousemove',e=>{
  const {x,y}=toWorld(e.clientX,e.clientY);
  let hov=null;
  for(const n of nodes){if(Math.hypot(n.x-x,n.y-y)<R/scale+4){hov=n;break;}}
  if(hov){
    document.getElementById('tn').textContent=hov.label;
    document.getElementById('tt').textContent=hov.type;
    document.getElementById('to').textContent=(hov.observations||[]).join('\n')||'(no observations)';
    tip.style.display='block';
    tip.style.left=(e.clientX+14)+'px';tip.style.top=(e.clientY-8)+'px';
  }else{tip.style.display='none';}
  if(dragging){ox+=(e.clientX-dragStart.x)/scale;oy+=(e.clientY-dragStart.y)/scale;dragStart={x:e.clientX,y:e.clientY};}
});
canvas.addEventListener('mouseleave',()=>{tip.style.display='none';});
canvas.addEventListener('mousedown',e=>{dragging=true;dragStart={x:e.clientX,y:e.clientY};});
canvas.addEventListener('mouseup',()=>{dragging=false;});
canvas.addEventListener('wheel',e=>{
  e.preventDefault();
  scale=Math.max(0.1,Math.min(10,scale*(e.deltaY<0?1.1:0.91)));
},{passive:false});

function loop(){tick();draw();requestAnimationFrame(loop);}
loop();
</script>
</body>
</html>`
