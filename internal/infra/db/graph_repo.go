// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

// GraphRepo is the GORM implementation of core/graph.GraphRepository.
type GraphRepo struct {
	db *gorm.DB
}

// NewGraphRepo creates a GraphRepo backed by db.
func NewGraphRepo(db *gorm.DB) *GraphRepo {
	return &GraphRepo{db: db}
}

// --- Private helpers ---

type edgeRow struct {
	FromNode     string `gorm:"column:from_node"`
	ToNode       string `gorm:"column:to_node"`
	RelationType string `gorm:"column:relation_type"`
	Confidence   string `gorm:"column:confidence"`
}

type pathEdgeRow struct {
	FromNode     string `gorm:"column:from_node"`
	ToNode       string `gorm:"column:to_node"`
	RelationType string `gorm:"column:relation_type"`
	Confidence   string `gorm:"column:confidence"`
}

type pathParentInfo struct {
	parent string
	edge   coregraph.PathEdge
}

var authoritativeSources = map[string]bool{
	"asyncapi": true,
	"proto":    true,
	"openapi":  true,
}

func (r *GraphRepo) loadGraph() (coregraph.KnowledgeGraph, error) {
	var dbEntities []dbEntity
	if err := r.db.Find(&dbEntities).Error; err != nil {
		return coregraph.KnowledgeGraph{}, err
	}

	var dbObs []dbObservation
	if err := r.db.Find(&dbObs).Error; err != nil {
		return coregraph.KnowledgeGraph{}, err
	}

	obsMap := make(map[string][]string)
	for _, obs := range dbObs {
		obsMap[obs.EntityName] = append(obsMap[obs.EntityName], obs.Content)
	}

	var entities []coregraph.Entity
	for _, e := range dbEntities {
		entities = append(entities, coregraph.Entity{
			Name:         e.Name,
			EntityType:   e.EntityType,
			Observations: obsMap[e.Name],
		})
	}

	var dbRels []dbRelation
	if err := r.db.Find(&dbRels).Error; err != nil {
		return coregraph.KnowledgeGraph{}, err
	}

	var relations []coregraph.Relation
	for _, rel := range dbRels {
		relations = append(relations, coregraph.Relation{
			From:         rel.FromEntity,
			To:           rel.ToEntity,
			RelationType: rel.RelationType,
			Confidence:   rel.Confidence,
		})
	}

	return coregraph.KnowledgeGraph{Entities: entities, Relations: relations}, nil
}

func (r *GraphRepo) buildSubGraph(entities []dbEntity) (coregraph.KnowledgeGraph, error) {
	if len(entities) == 0 {
		return coregraph.KnowledgeGraph{}, nil
	}

	names := make([]string, 0, len(entities))
	for _, e := range entities {
		names = append(names, e.Name)
	}

	var dbObs []dbObservation
	if err := r.db.Where("entity_name IN ?", names).Find(&dbObs).Error; err != nil {
		return coregraph.KnowledgeGraph{}, err
	}

	obsMap := make(map[string][]string)
	for _, obs := range dbObs {
		obsMap[obs.EntityName] = append(obsMap[obs.EntityName], obs.Content)
	}

	var resultEntities []coregraph.Entity
	for _, e := range entities {
		resultEntities = append(resultEntities, coregraph.Entity{
			Name:         e.Name,
			EntityType:   e.EntityType,
			Observations: obsMap[e.Name],
		})
	}

	var dbRels []dbRelation
	if err := r.db.Where("from_node IN ? AND to_node IN ?", names, names).Find(&dbRels).Error; err != nil {
		return coregraph.KnowledgeGraph{}, err
	}

	var resultRels []coregraph.Relation
	for _, rel := range dbRels {
		resultRels = append(resultRels, coregraph.Relation{
			From:         rel.FromEntity,
			To:           rel.ToEntity,
			RelationType: rel.RelationType,
			Confidence:   rel.Confidence,
		})
	}

	return coregraph.KnowledgeGraph{Entities: resultEntities, Relations: resultRels}, nil
}

func (r *GraphRepo) edgeQuery(frontier []string, relationType, direction string) ([]edgeRow, error) {
	var rows []edgeRow

	db := r.db.Model(&dbRelation{})
	if direction == "outgoing" {
		db = db.Select("from_node, to_node, relation_type, confidence").Where("from_node IN ?", frontier)
	} else {
		db = db.Select("to_node AS from_node, from_node AS to_node, relation_type, confidence").Where("to_node IN ?", frontier)
	}
	if relationType != "" {
		db = db.Where("relation_type = ?", relationType)
	}
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *GraphRepo) queryNeighbours(frontier []string, relationType, direction string) ([]edgeRow, error) {
	var result []edgeRow

	if direction == "outgoing" || direction == "both" {
		rows, err := r.edgeQuery(frontier, relationType, "outgoing")
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
	}

	if direction == "incoming" || direction == "both" {
		rows, err := r.edgeQuery(frontier, relationType, "incoming")
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
	}

	return result, nil
}

func (r *GraphRepo) allEdgesFor(frontier []string) ([]pathEdgeRow, error) {
	var rows []pathEdgeRow
	err := r.db.Model(&dbRelation{}).
		Select("from_node, to_node, relation_type, confidence").
		Where("from_node IN ? OR to_node IN ?", frontier, frontier).
		Find(&rows).Error
	return rows, err
}

func reconstructPath(parent map[string]pathParentInfo, from, to string) []coregraph.PathEdge {
	var path []coregraph.PathEdge
	cur := to
	for cur != from {
		info := parent[cur]
		path = append(path, info.edge)
		cur = info.parent
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// --- GraphRepository interface implementation ---

func (r *GraphRepo) CreateEntities(entities []coregraph.Entity) ([]coregraph.Entity, error) {
	var newEntities []coregraph.Entity
	for _, e := range entities {
		var count int64
		if err := r.db.Model(&dbEntity{}).Where("name = ?", e.Name).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			if err := r.db.Create(&dbEntity{Name: e.Name, EntityType: e.EntityType}).Error; err != nil {
				return nil, err
			}
			for _, obs := range e.Observations {
				if err := r.db.Create(&dbObservation{EntityName: e.Name, Content: obs}).Error; err != nil {
					return nil, err
				}
			}
			newEntities = append(newEntities, e)
		}
	}
	return newEntities, nil
}

func (r *GraphRepo) CreateRelations(relations []coregraph.Relation) ([]coregraph.Relation, error) {
	var newRelations []coregraph.Relation
	for _, rel := range relations {
		var count int64
		if err := r.db.Model(&dbRelation{}).Where("from_node = ? AND to_node = ? AND relation_type = ?", rel.From, rel.To, rel.RelationType).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			confidence := rel.Confidence
			if confidence == "" {
				confidence = "authoritative"
			}
			if err := r.db.Create(&dbRelation{FromEntity: rel.From, ToEntity: rel.To, RelationType: rel.RelationType, Confidence: confidence}).Error; err != nil {
				return nil, err
			}
			newRelations = append(newRelations, coregraph.Relation{From: rel.From, To: rel.To, RelationType: rel.RelationType, Confidence: confidence})
		}
	}
	return newRelations, nil
}

func (r *GraphRepo) AddObservations(observations []coregraph.Observation) ([]coregraph.Observation, error) {
	var results []coregraph.Observation
	for _, obs := range observations {
		var count int64
		if err := r.db.Model(&dbEntity{}).Where("name = ?", obs.EntityName).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("entity with name %s not found", obs.EntityName)
		}
		var newObs []string
		for _, content := range obs.Contents {
			var existingCount int64
			if err := r.db.Model(&dbObservation{}).Where("entity_name = ? AND content = ?", obs.EntityName, content).Count(&existingCount).Error; err != nil {
				return nil, err
			}
			if existingCount == 0 {
				if err := r.db.Create(&dbObservation{EntityName: obs.EntityName, Content: content}).Error; err != nil {
					return nil, err
				}
				newObs = append(newObs, content)
			}
		}
		if len(newObs) > 0 {
			results = append(results, coregraph.Observation{EntityName: obs.EntityName, Contents: newObs})
		}
	}
	return results, nil
}

func (r *GraphRepo) DeleteEntities(entityNames []string) error {
	if len(entityNames) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("name IN ?", entityNames).Delete(&dbEntity{}).Error; err != nil {
			return err
		}
		if err := tx.Where("entity_name IN ?", entityNames).Delete(&dbObservation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("from_node IN ? OR to_node IN ?", entityNames, entityNames).Delete(&dbRelation{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *GraphRepo) DeleteObservations(deletions []coregraph.Observation) error {
	for _, d := range deletions {
		if len(d.Observations) == 0 {
			continue
		}
		if err := r.db.Where("entity_name = ? AND content IN ?", d.EntityName, d.Observations).Delete(&dbObservation{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *GraphRepo) DeleteRelations(relations []coregraph.Relation) error {
	for _, rel := range relations {
		if err := r.db.Where("from_node = ? AND to_node = ? AND relation_type = ?", rel.From, rel.To, rel.RelationType).Delete(&dbRelation{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *GraphRepo) ReadGraph() (coregraph.KnowledgeGraph, error) {
	return r.loadGraph()
}

func (r *GraphRepo) SearchNodes(query string) (coregraph.KnowledgeGraph, error) {
	return r.SearchNodesFiltered(query, false)
}

func (r *GraphRepo) SearchNodesFiltered(query string, includeArchived bool) (coregraph.KnowledgeGraph, error) {
	queryPattern := "%" + strings.ToLower(query) + "%"
	var matchingEntities []dbEntity
	var err error

	if includeArchived {
		err = r.db.Raw(`
			SELECT DISTINCT e.* FROM db_entities e
			LEFT JOIN db_observations o ON e.name = o.entity_name
			WHERE LOWER(e.name) LIKE ? OR LOWER(e.entity_type) LIKE ? OR LOWER(o.content) LIKE ?
		`, queryPattern, queryPattern, queryPattern).Scan(&matchingEntities).Error
	} else {
		err = r.db.Raw(`
			SELECT DISTINCT e.* FROM db_entities e
			LEFT JOIN db_observations o ON e.name = o.entity_name
			WHERE (LOWER(e.name) LIKE ? OR LOWER(e.entity_type) LIKE ? OR LOWER(o.content) LIKE ?)
			  AND e.name NOT IN (
			    SELECT entity_name FROM db_observations WHERE content = '_status:archived'
			  )
		`, queryPattern, queryPattern, queryPattern).Scan(&matchingEntities).Error
	}

	if err != nil {
		return coregraph.KnowledgeGraph{}, err
	}
	return r.buildSubGraph(matchingEntities)
}

func (r *GraphRepo) OpenNodes(names []string) (coregraph.KnowledgeGraph, error) {
	return r.OpenNodesFiltered(names, false)
}

func (r *GraphRepo) OpenNodesFiltered(names []string, includeArchived bool) (coregraph.KnowledgeGraph, error) {
	if len(names) == 0 {
		return coregraph.KnowledgeGraph{}, nil
	}

	var matchingEntities []dbEntity
	var err error

	if includeArchived {
		err = r.db.Where("name IN ?", names).Find(&matchingEntities).Error
	} else {
		err = r.db.Raw(`
			SELECT e.* FROM db_entities e
			WHERE e.name IN ?
			  AND e.name NOT IN (
			    SELECT entity_name FROM db_observations WHERE content = '_status:archived'
			  )
		`, names).Scan(&matchingEntities).Error
	}

	if err != nil {
		return coregraph.KnowledgeGraph{}, err
	}
	return r.buildSubGraph(matchingEntities)
}

func (r *GraphRepo) EntityCount() (int64, error) {
	var count int64
	err := r.db.Model(&dbEntity{}).Count(&count).Error
	return count, err
}

func (r *GraphRepo) EntityTypeCounts() (map[string]int64, error) {
	type row struct {
		EntityType string
		Count      int64
	}
	var rows []row
	if err := r.db.Model(&dbEntity{}).
		Select("entity_type, COUNT(*) AS count").
		Group("entity_type").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.EntityType] = row.Count
	}
	return counts, nil
}

func (r *GraphRepo) ListEntities(entityType string) (coregraph.KnowledgeGraph, error) {
	var entities []dbEntity
	query := r.db.Model(&dbEntity{})
	if entityType != "" {
		query = query.Where("LOWER(entity_type) = LOWER(?)", entityType)
	}
	if err := query.Find(&entities).Error; err != nil {
		return coregraph.KnowledgeGraph{}, err
	}
	return r.buildSubGraph(entities)
}

func (r *GraphRepo) ListRelations(relationType, fromEntity string) ([]coregraph.Relation, error) {
	query := r.db.Model(&dbRelation{})
	if relationType != "" {
		query = query.Where("relation_type = ?", relationType)
	}
	if fromEntity != "" {
		query = query.Where("from_node = ?", fromEntity)
	}
	var rows []dbRelation
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	relations := make([]coregraph.Relation, len(rows))
	for i, rel := range rows {
		relations[i] = coregraph.Relation{From: rel.FromEntity, To: rel.ToEntity, RelationType: rel.RelationType, Confidence: rel.Confidence}
	}
	return relations, nil
}

func (r *GraphRepo) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]coregraph.TraverseNode, []coregraph.TraverseEdge, error) {
	type bfsEntry struct {
		name string
		path []string
	}

	visited := map[string]bool{entity: true}
	frontier := []bfsEntry{{name: entity, path: []string{entity}}}
	var results []coregraph.TraverseNode
	var edges []coregraph.TraverseEdge

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		frontierNames := make([]string, len(frontier))
		for i, e := range frontier {
			frontierNames[i] = e.name
		}

		pathOf := make(map[string][]string, len(frontier))
		for _, e := range frontier {
			pathOf[e.name] = e.path
		}

		edgeRows, err := r.queryNeighbours(frontierNames, relationType, direction)
		if err != nil {
			return nil, nil, err
		}

		var nextFrontier []bfsEntry
		for _, row := range edgeRows {
			parentPath := pathOf[row.FromNode]
			target := row.ToNode
			edges = append(edges, coregraph.TraverseEdge{
				From:         row.FromNode,
				To:           target,
				RelationType: row.RelationType,
				Confidence:   row.Confidence,
			})
			if visited[target] {
				continue
			}
			visited[target] = true
			nodePath := make([]string, len(parentPath), len(parentPath)+1)
			copy(nodePath, parentPath)
			nodePath = append(nodePath, target)

			results = append(results, coregraph.TraverseNode{
				Name:     target,
				Distance: depth,
				Path:     nodePath[1:],
			})
			nextFrontier = append(nextFrontier, bfsEntry{name: target, path: nodePath})
		}
		frontier = nextFrontier
	}

	if len(results) == 0 {
		return results, edges, nil
	}

	names := make([]string, len(results))
	for i, n := range results {
		names[i] = n.Name
	}

	var dbEntities []dbEntity
	if err := r.db.Where("name IN ?", names).Find(&dbEntities).Error; err != nil {
		return nil, nil, err
	}
	typeOf := make(map[string]string, len(dbEntities))
	for _, e := range dbEntities {
		typeOf[e.Name] = e.EntityType
	}

	var dbObs []dbObservation
	if err := r.db.Where("entity_name IN ?", names).Find(&dbObs).Error; err != nil {
		return nil, nil, err
	}
	obsOf := make(map[string][]string)
	for _, o := range dbObs {
		obsOf[o.EntityName] = append(obsOf[o.EntityName], o.Content)
	}

	for i := range results {
		results[i].EntityType = typeOf[results[i].Name]
		results[i].Observations = obsOf[results[i].Name]
	}

	return results, edges, nil
}

func (r *GraphRepo) GetIntegrationMap(_ context.Context, service string, depth int) (coregraph.IntegrationMap, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	result := coregraph.IntegrationMap{Service: service}

	var obs []dbObservation
	if err := r.db.Where("entity_name = ?", service).Find(&obs).Error; err != nil {
		return result, err
	}

	integrationSources := make(map[string]bool)
	for _, o := range obs {
		if src, ok := strings.CutPrefix(o.Content, "_integration_source:"); ok {
			integrationSources[src] = true
		}
	}

	confidence := func(source string) string {
		if authoritativeSources[source] {
			return "authoritative"
		}
		return "inferred"
	}

	hasAuthoritative := false
	hasInferred := false
	for src := range integrationSources {
		if authoritativeSources[src] {
			hasAuthoritative = true
		} else {
			hasInferred = true
		}
	}

	integrationRelTypes := []struct {
		relType  string
		assignTo func([]coregraph.IntegrationEdge)
		source   string
	}{
		{"publishes_event", func(e []coregraph.IntegrationEdge) { result.Publishes = e }, "asyncapi"},
		{"subscribes_event", func(e []coregraph.IntegrationEdge) { result.Subscribes = e }, "asyncapi"},
		{"exposes_api", func(e []coregraph.IntegrationEdge) { result.ExposesAPI = e }, "openapi"},
		{"provides_grpc", func(e []coregraph.IntegrationEdge) { result.ProvidesGRPC = e }, "proto"},
		{"depends_on_grpc", func(e []coregraph.IntegrationEdge) { result.GRPCDeps = e }, "proto"},
		{"calls_service", func(e []coregraph.IntegrationEdge) { result.Calls = e }, "k8s-env"},
	}

	hasAnyRelations := false
	for _, rt := range integrationRelTypes {
		var rels []dbRelation
		if err := r.db.Where("from_node = ? AND relation_type = ?", service, rt.relType).Find(&rels).Error; err != nil {
			return result, err
		}
		if len(rels) == 0 {
			continue
		}
		hasAnyRelations = true
		conf := confidence(rt.source)
		edges := make([]coregraph.IntegrationEdge, 0, len(rels))
		for _, rel := range rels {
			edges = append(edges, coregraph.IntegrationEdge{
				Target:     rel.ToEntity,
				Confidence: conf,
			})
		}
		rt.assignTo(edges)
	}

	switch {
	case !hasAnyRelations && len(integrationSources) == 0:
		result.Coverage = "none"
	case hasAuthoritative && !hasInferred:
		result.Coverage = "full"
	case hasAuthoritative && hasInferred:
		result.Coverage = "partial"
	case !hasAuthoritative && hasInferred:
		result.Coverage = "inferred"
	default:
		result.Coverage = "none"
	}

	return result, nil
}

func (r *GraphRepo) FindPath(from, to string, maxDepth int) ([]coregraph.PathEdge, error) {
	if maxDepth < 1 {
		maxDepth = 1
	}
	if maxDepth > 10 {
		maxDepth = 10
	}
	if from == to {
		return []coregraph.PathEdge{}, nil
	}

	visited := map[string]bool{from: true}
	parent := map[string]pathParentInfo{}
	frontier := []string{from}

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		edges, err := r.allEdgesFor(frontier)
		if err != nil {
			return nil, err
		}

		var nextFrontier []string
		for _, e := range edges {
			candidates := []struct {
				frontierNode, neighbour string
				edge                    coregraph.PathEdge
			}{
				{e.FromNode, e.ToNode, coregraph.PathEdge{From: e.FromNode, RelationType: e.RelationType, To: e.ToNode, Confidence: e.Confidence}},
				{e.ToNode, e.FromNode, coregraph.PathEdge{From: e.FromNode, RelationType: e.RelationType, To: e.ToNode, Confidence: e.Confidence}},
			}

			for _, c := range candidates {
				if !visited[c.frontierNode] {
					continue
				}
				if visited[c.neighbour] {
					continue
				}
				visited[c.neighbour] = true
				parent[c.neighbour] = pathParentInfo{parent: c.frontierNode, edge: c.edge}

				if c.neighbour == to {
					return reconstructPath(parent, from, to), nil
				}
				nextFrontier = append(nextFrontier, c.neighbour)
			}
		}
		frontier = nextFrontier
	}

	return []coregraph.PathEdge{}, nil
}

func (r *GraphRepo) UpdateEntity(oldName, newName, newType string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var entity dbEntity
		if err := tx.Where("name = ?", oldName).First(&entity).Error; err != nil {
			return fmt.Errorf("entity %q not found: %w", oldName, err)
		}

		if newName != "" && newName != oldName {
			var taken int64
			if err := tx.Model(&dbEntity{}).Where("name = ?", newName).Count(&taken).Error; err != nil {
				return fmt.Errorf("checking entity name %q: %w", newName, err)
			}
			if taken > 0 {
				return fmt.Errorf("entity %q already exists", newName)
			}

			if err := tx.Model(&dbRelation{}).Where("from_node = ?", oldName).Update("from_node", newName).Error; err != nil {
				return err
			}
			if err := tx.Model(&dbRelation{}).Where("to_node = ?", oldName).Update("to_node", newName).Error; err != nil {
				return err
			}
			if err := tx.Model(&dbObservation{}).Where("entity_name = ?", oldName).Update("entity_name", newName).Error; err != nil {
				return err
			}
			if err := tx.Model(&dbEntity{}).Where("name = ?", oldName).Update("name", newName).Error; err != nil {
				return err
			}
			oldName = newName
		}

		if newType != "" && newType != entity.EntityType {
			if err := tx.Model(&dbEntity{}).Where("name = ?", oldName).Update("entity_type", newType).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
