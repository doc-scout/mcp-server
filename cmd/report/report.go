// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

var reNonWord = regexp.MustCompile(`[^a-zA-Z0-9]`)

// graphStats is the subset of graph service methods used by the report tool.
type graphStats interface {
	EntityCount() (int64, error)
	EntityTypeCounts() (map[string]int64, error)
}

// nodeID converts an entity name into a valid Mermaid node identifier.
func nodeID(name string) string {
	return "n_" + reNonWord.ReplaceAllString(name, "_")
}

// mermaidShape returns a Mermaid node declaration with shape based on entity type.
func mermaidShape(name, entityType string) string {
	id := nodeID(name)
	label := fmt.Sprintf(`%s\n%s`, name, entityType)
	switch entityType {
	case "api":
		return fmt.Sprintf(`%s(["%s"])`, id, label)
	case "team":
		return fmt.Sprintf(`%s(("%s"))`, id, label)
	case "grpc-service":
		return fmt.Sprintf(`%s[("%s")]`, id, label)
	default:
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

// buildPie generates a Mermaid pie chart block from entity type counts.
// Returns empty string when counts is nil or empty.
func buildPie(counts map[string]int64) string {
	if len(counts) == 0 {
		return ""
	}
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
func buildFlowchart(nodes []TopNode, edges []TopEdge, totalNodes, totalEdges int) string {
	if len(nodes) == 0 {
		return ""
	}

	var b strings.Builder

	if len(nodes) < totalNodes || len(edges) < totalEdges {
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
func queryTopNodes(db *gorm.DB, maxNodes int) ([]TopNode, int, error) {
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
		nodes[i] = TopNode(r)
	}
	return nodes, int(totalCount), nil
}

// queryEdges returns edges between the given nodes (both endpoints in the set),
// up to maxEdges, plus the total edge count in the subgraph before truncation.
func queryEdges(db *gorm.DB, nodes []TopNode, maxEdges int) ([]TopEdge, int, error) {
	if len(nodes) == 0 {
		return nil, 0, nil
	}

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

// countRelations returns the total number of relations in the DB.
func countRelations(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Table("db_relations").Count(&count).Error
	return count, err
}

// ReportConfig holds the parameters for GenerateReport.
type ReportConfig struct {
	Repo     string
	Elapsed  int
	MaxNodes int
	MaxEdges int
}

// GenerateReport builds the full Markdown+Mermaid report string.
func GenerateReport(svc graphStats, db *gorm.DB, cfg ReportConfig) (string, error) {
	typeCounts, err := svc.EntityTypeCounts()
	if err != nil {
		return "", fmt.Errorf("entity type counts: %w", err)
	}

	totalEntities, err := svc.EntityCount()
	if err != nil {
		return "", fmt.Errorf("entity count: %w", err)
	}

	totalRelations, err := countRelations(db)
	if err != nil {
		return "", fmt.Errorf("relation count: %w", err)
	}

	nodes, totalNodes, err := queryTopNodes(db, cfg.MaxNodes)
	if err != nil {
		return "", fmt.Errorf("top nodes: %w", err)
	}

	edges, totalEdges, err := queryEdges(db, nodes, cfg.MaxEdges)
	if err != nil {
		return "", fmt.Errorf("edges: %w", err)
	}

	var b strings.Builder
	b.WriteString("## DocScout Graph Analysis\n\n")
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

	if chart := buildFlowchart(nodes, edges, totalNodes, totalEdges); chart != "" {
		b.WriteString("### Service Topology\n\n")
		b.WriteString(chart)
		b.WriteString("\n")
	}

	return b.String(), nil
}
