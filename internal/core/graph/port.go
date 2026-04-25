// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package graph

import "context"

// GraphRepository is the outbound port implemented by internal/infra/db.
type GraphRepository interface {
	CreateEntities([]Entity) ([]Entity, error)
	CreateRelations([]Relation) ([]Relation, error)
	AddObservations([]Observation) ([]Observation, error)
	DeleteEntities(entityNames []string) error
	DeleteObservations(deletions []Observation) error
	DeleteRelations(relations []Relation) error
	ReadGraph() (KnowledgeGraph, error)
	SearchNodes(query string) (KnowledgeGraph, error)
	SearchNodesFiltered(query string, includeArchived bool) (KnowledgeGraph, error)
	OpenNodes(names []string) (KnowledgeGraph, error)
	OpenNodesFiltered(names []string, includeArchived bool) (KnowledgeGraph, error)
	EntityCount() (int64, error)
	EntityTypeCounts() (map[string]int64, error)
	ListEntities(entityType string) (KnowledgeGraph, error)
	ListRelations(relationType, fromEntity string) ([]Relation, error)
	TraverseGraph(entity, relationType, direction string, maxDepth int) ([]TraverseNode, []TraverseEdge, error)
	GetIntegrationMap(ctx context.Context, service string, depth int) (IntegrationMap, error)
	FindPath(from, to string, maxDepth int) ([]PathEdge, error)
	UpdateEntity(oldName, newName, newType string) error
}

// GraphService is the inbound port called by internal/adapter/mcp and decorated by
// the audit logger in internal/app. Embedding GraphRepository allows decoration
// without re-listing every method.
type GraphService interface {
	GraphRepository
}
