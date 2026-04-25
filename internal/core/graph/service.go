// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package graph

import "context"

// MemoryService implements GraphService by delegating to a GraphRepository.
type MemoryService struct {
	repo GraphRepository
}

// NewMemoryService creates a MemoryService backed by repo.
func NewMemoryService(repo GraphRepository) *MemoryService {
	return &MemoryService{repo: repo}
}

func (srv *MemoryService) CreateEntities(entities []Entity) ([]Entity, error) {
	return srv.repo.CreateEntities(entities)
}

func (srv *MemoryService) CreateRelations(relations []Relation) ([]Relation, error) {
	return srv.repo.CreateRelations(relations)
}

func (srv *MemoryService) AddObservations(obs []Observation) ([]Observation, error) {
	return srv.repo.AddObservations(obs)
}

func (srv *MemoryService) DeleteEntities(names []string) error {
	return srv.repo.DeleteEntities(names)
}

func (srv *MemoryService) DeleteObservations(deletions []Observation) error {
	return srv.repo.DeleteObservations(deletions)
}

func (srv *MemoryService) DeleteRelations(relations []Relation) error {
	return srv.repo.DeleteRelations(relations)
}

func (srv *MemoryService) ReadGraph() (KnowledgeGraph, error) {
	return srv.repo.ReadGraph()
}

func (srv *MemoryService) SearchNodes(query string) (KnowledgeGraph, error) {
	return srv.repo.SearchNodes(query)
}

func (srv *MemoryService) SearchNodesFiltered(query string, includeArchived bool) (KnowledgeGraph, error) {
	return srv.repo.SearchNodesFiltered(query, includeArchived)
}

func (srv *MemoryService) OpenNodes(names []string) (KnowledgeGraph, error) {
	return srv.repo.OpenNodes(names)
}

func (srv *MemoryService) OpenNodesFiltered(names []string, includeArchived bool) (KnowledgeGraph, error) {
	return srv.repo.OpenNodesFiltered(names, includeArchived)
}

func (srv *MemoryService) EntityCount() (int64, error) {
	return srv.repo.EntityCount()
}

func (srv *MemoryService) EntityTypeCounts() (map[string]int64, error) {
	return srv.repo.EntityTypeCounts()
}

func (srv *MemoryService) ListEntities(entityType string) (KnowledgeGraph, error) {
	return srv.repo.ListEntities(entityType)
}

func (srv *MemoryService) ListRelations(relationType, fromEntity string) ([]Relation, error) {
	return srv.repo.ListRelations(relationType, fromEntity)
}

func (srv *MemoryService) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]TraverseNode, []TraverseEdge, error) {
	return srv.repo.TraverseGraph(entity, relationType, direction, maxDepth)
}

func (srv *MemoryService) GetIntegrationMap(ctx context.Context, service string, depth int) (IntegrationMap, error) {
	return srv.repo.GetIntegrationMap(ctx, service, depth)
}

func (srv *MemoryService) FindPath(from, to string, maxDepth int) ([]PathEdge, error) {
	return srv.repo.FindPath(from, to, maxDepth)
}

func (srv *MemoryService) UpdateEntity(oldName, newName, newType string) error {
	return srv.repo.UpdateEntity(oldName, newName, newType)
}
