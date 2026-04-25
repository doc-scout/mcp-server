// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package graph

// Entity represents a knowledge graph node with observations.
type Entity struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
}

// Relation represents a directed edge between two entities.
type Relation struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
	// Confidence indicates how the relation was derived.
	// "authoritative" — from an explicit contract file.
	// "inferred"      — from a config heuristic.
	// "ambiguous"     — caller explicitly marked the edge as uncertain.
	Confidence string `json:"confidence,omitempty"`
}

// Observation contains facts about an entity.
type Observation struct {
	EntityName   string   `json:"entityName"`
	Contents     []string `json:"contents"`
	Observations []string `json:"observations,omitempty"` // For deletion
}

// KnowledgeGraph represents the complete graph structure.
type KnowledgeGraph struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}

// TraverseNode is a node reached during graph traversal, enriched with
// distance from the start entity and the path of entity names leading to it.
type TraverseNode struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
	Distance     int      `json:"distance"`
	Path         []string `json:"path"` // entity names from start (exclusive) to this node (inclusive)
}

// TraverseEdge is a directed edge discovered during graph traversal.
type TraverseEdge struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
	Confidence   string `json:"confidence,omitempty"`
}

// PathEdge is a single directed edge on the path between two entities.
// The edge reflects the actual stored direction regardless of traversal direction.
type PathEdge struct {
	From         string `json:"from"`
	RelationType string `json:"relationType"`
	To           string `json:"to"`
	Confidence   string `json:"confidence,omitempty"`
}

// IntegrationEdge is a single integration relationship entry in an IntegrationMap.
type IntegrationEdge struct {
	Target     string `json:"target"`
	Schema     string `json:"schema,omitempty"`      // event-topic schema name
	Version    string `json:"version,omitempty"`     // api version
	Paths      int    `json:"paths,omitempty"`       // api path count
	Confidence string `json:"confidence"`            // "authoritative" | "inferred"
	SourceRepo string `json:"source_repo,omitempty"` // originating repo
}

// IntegrationMap aggregates all integration relationships for a service.
type IntegrationMap struct {
	Service      string            `json:"service"`
	Publishes    []IntegrationEdge `json:"publishes"`
	Subscribes   []IntegrationEdge `json:"subscribes"`
	ExposesAPI   []IntegrationEdge `json:"exposes_api"`
	ProvidesGRPC []IntegrationEdge `json:"provides_grpc"`
	GRPCDeps     []IntegrationEdge `json:"grpc_deps"`
	Calls        []IntegrationEdge `json:"calls"`
	Coverage     string            `json:"graph_coverage"` // "full" | "partial" | "inferred" | "none"
}
