// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// inMemoryCounter is used to generate unique in-memory SQLite database names,

// ensuring each call to OpenDB("") creates an isolated database.

var inMemoryCounter atomic.Int64

// --- API Structs (JSON + MCP Schema) ---

// Entity represents a knowledge graph node with observations.

type Entity struct {
	Name string `json:"name"`

	EntityType string `json:"entityType"`

	Observations []string `json:"observations"`
}

// Relation represents a directed edge between two entities.

type Relation struct {
	From string `json:"from"`

	To string `json:"to"`

	RelationType string `json:"relationType"`
}

// Observation contains facts about an entity.

type Observation struct {
	EntityName string `json:"entityName"`

	Contents []string `json:"contents"`

	Observations []string `json:"observations,omitempty"` // For deletion

}

// KnowledgeGraph represents the complete graph structure.

type KnowledgeGraph struct {
	Entities []Entity `json:"entities"`

	Relations []Relation `json:"relations"`
}

// --- GORM DB Models ---

type dbEntity struct {
	Name string `gorm:"primaryKey"`

	EntityType string `gorm:"index"`
}

type dbRelation struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	FromEntity string `gorm:"index;column:from_node"`

	ToEntity string `gorm:"index;column:to_node"`

	RelationType string `gorm:"index"`
}

type dbObservation struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	EntityName string `gorm:"index;column:entity_name"`

	Content string
}

// --- Core Store ---

type store struct {
	db *gorm.DB
}

func (s store) loadGraph() (KnowledgeGraph, error) {

	var dbEntities []dbEntity

	if err := s.db.Find(&dbEntities).Error; err != nil {

		return KnowledgeGraph{}, err

	}

	var dbObs []dbObservation

	if err := s.db.Find(&dbObs).Error; err != nil {

		return KnowledgeGraph{}, err

	}

	obsMap := make(map[string][]string)

	for _, obs := range dbObs {

		obsMap[obs.EntityName] = append(obsMap[obs.EntityName], obs.Content)

	}

	var entities []Entity

	for _, e := range dbEntities {

		entities = append(entities, Entity{

			Name: e.Name,

			EntityType: e.EntityType,

			Observations: obsMap[e.Name],
		})

	}

	var dbRels []dbRelation

	if err := s.db.Find(&dbRels).Error; err != nil {

		return KnowledgeGraph{}, err

	}

	var relations []Relation

	for _, r := range dbRels {

		relations = append(relations, Relation{

			From: r.FromEntity,

			To: r.ToEntity,

			RelationType: r.RelationType,
		})

	}

	return KnowledgeGraph{Entities: entities, Relations: relations}, nil

}

func (s store) createEntities(entities []Entity) ([]Entity, error) {

	var newEntities []Entity

	for _, e := range entities {

		var count int64

		if err := s.db.Model(&dbEntity{}).Where("name = ?", e.Name).Count(&count).Error; err != nil {

			return nil, err

		}

		if count == 0 {

			if err := s.db.Create(&dbEntity{Name: e.Name, EntityType: e.EntityType}).Error; err != nil {

				return nil, err

			}

			for _, obs := range e.Observations {

				if err := s.db.Create(&dbObservation{EntityName: e.Name, Content: obs}).Error; err != nil {

					return nil, err

				}

			}

			newEntities = append(newEntities, e)

		}

	}

	return newEntities, nil

}

func (s store) createRelations(relations []Relation) ([]Relation, error) {

	var newRelations []Relation

	for _, r := range relations {

		var count int64

		if err := s.db.Model(&dbRelation{}).Where("from_node = ? AND to_node = ? AND relation_type = ?", r.From, r.To, r.RelationType).Count(&count).Error; err != nil {

			return nil, err

		}

		if count == 0 {

			if err := s.db.Create(&dbRelation{FromEntity: r.From, ToEntity: r.To, RelationType: r.RelationType}).Error; err != nil {

				return nil, err

			}

			newRelations = append(newRelations, r)

		}

	}

	return newRelations, nil

}

func (s store) addObservations(observations []Observation) ([]Observation, error) {

	var results []Observation

	for _, obs := range observations {

		var count int64

		if err := s.db.Model(&dbEntity{}).Where("name = ?", obs.EntityName).Count(&count).Error; err != nil {

			return nil, err

		}

		if count == 0 {

			return nil, fmt.Errorf("entity with name %s not found", obs.EntityName)

		}

		var newObs []string

		for _, content := range obs.Contents {

			var existingCount int64

			if err := s.db.Model(&dbObservation{}).Where("entity_name = ? AND content = ?", obs.EntityName, content).Count(&existingCount).Error; err != nil {

				return nil, err

			}

			if existingCount == 0 {

				if err := s.db.Create(&dbObservation{EntityName: obs.EntityName, Content: content}).Error; err != nil {

					return nil, err

				}

				newObs = append(newObs, content)

			}

		}

		if len(newObs) > 0 {

			results = append(results, Observation{EntityName: obs.EntityName, Contents: newObs})

		}

	}

	return results, nil

}

func (s store) deleteEntities(entityNames []string) error {

	if len(entityNames) == 0 {

		return nil

	}

	return s.db.Transaction(func(tx *gorm.DB) error {

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

func (s store) deleteObservations(deletions []Observation) error {

	for _, d := range deletions {

		if len(d.Observations) == 0 {

			continue

		}

		if err := s.db.Where("entity_name = ? AND content IN ?", d.EntityName, d.Observations).Delete(&dbObservation{}).Error; err != nil {

			return err

		}

	}

	return nil

}

func (s store) deleteRelations(relations []Relation) error {

	for _, r := range relations {

		if err := s.db.Where("from_node = ? AND to_node = ? AND relation_type = ?", r.From, r.To, r.RelationType).Delete(&dbRelation{}).Error; err != nil {

			return err

		}

	}

	return nil

}

func (s store) listRelations(relationType, fromEntity string) ([]Relation, error) {

	query := s.db.Model(&dbRelation{})

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

	relations := make([]Relation, len(rows))

	for i, r := range rows {

		relations[i] = Relation{From: r.FromEntity, To: r.ToEntity, RelationType: r.RelationType}

	}

	return relations, nil

}

// updateEntity renames an entity and/or changes its type in a transaction.

// At least one of newName or newType must be non-empty.

// If newName is provided, all relation and observation references are updated atomically.

func (s store) updateEntity(oldName, newName, newType string) error {

	return s.db.Transaction(func(tx *gorm.DB) error {

		// Verify the entity exists.

		var entity dbEntity

		if err := tx.Where("name = ?", oldName).First(&entity).Error; err != nil {

			return fmt.Errorf("entity %q not found: %w", oldName, err)

		}

		if newName != "" && newName != oldName {

			// Ensure the new name is not already taken.

			var taken int64

			if err := tx.Model(&dbEntity{}).Where("name = ?", newName).Count(&taken).Error; err != nil {

				return fmt.Errorf("checking entity name %q: %w", newName, err)

			}

			if taken > 0 {

				return fmt.Errorf("entity %q already exists", newName)

			}

			// Update relations.

			if err := tx.Model(&dbRelation{}).Where("from_node = ?", oldName).Update("from_node", newName).Error; err != nil {

				return err

			}

			if err := tx.Model(&dbRelation{}).Where("to_node = ?", oldName).Update("to_node", newName).Error; err != nil {

				return err

			}

			// Update observations.

			if err := tx.Model(&dbObservation{}).Where("entity_name = ?", oldName).Update("entity_name", newName).Error; err != nil {

				return err

			}

			// Rename the entity (change primary key).

			if err := tx.Model(&dbEntity{}).Where("name = ?", oldName).Update("name", newName).Error; err != nil {

				return err

			}

			// For subsequent type update, work on the new name.

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

func (s store) searchNodes(query string) (KnowledgeGraph, error) {

	return s.searchNodesFiltered(query, false)

}

// searchNodesFiltered searches entities matching query.

// When includeArchived is false (default), entities with _status:archived are excluded.

func (s store) searchNodesFiltered(query string, includeArchived bool) (KnowledgeGraph, error) {

	queryPattern := "%" + strings.ToLower(query) + "%"

	var matchingEntities []dbEntity

	var err error

	if includeArchived {

		err = s.db.Raw(`

			SELECT DISTINCT e.* FROM db_entities e

			LEFT JOIN db_observations o ON e.name = o.entity_name

			WHERE LOWER(e.name) LIKE ? OR LOWER(e.entity_type) LIKE ? OR LOWER(o.content) LIKE ?

		`, queryPattern, queryPattern, queryPattern).Scan(&matchingEntities).Error

	} else {

		err = s.db.Raw(`

			SELECT DISTINCT e.* FROM db_entities e

			LEFT JOIN db_observations o ON e.name = o.entity_name

			WHERE (LOWER(e.name) LIKE ? OR LOWER(e.entity_type) LIKE ? OR LOWER(o.content) LIKE ?)

			  AND e.name NOT IN (

			    SELECT entity_name FROM db_observations WHERE content = '_status:archived'

			  )

		`, queryPattern, queryPattern, queryPattern).Scan(&matchingEntities).Error

	}

	if err != nil {

		return KnowledgeGraph{}, err

	}

	return s.buildSubGraph(matchingEntities)

}

func (s store) listEntities(entityType string) (KnowledgeGraph, error) {

	var entities []dbEntity

	query := s.db.Model(&dbEntity{})

	if entityType != "" {

		query = query.Where("LOWER(entity_type) = LOWER(?)", entityType)

	}

	if err := query.Find(&entities).Error; err != nil {

		return KnowledgeGraph{}, err

	}

	return s.buildSubGraph(entities)

}

func (s store) openNodes(names []string) (KnowledgeGraph, error) {

	return s.openNodesFiltered(names, false)

}

// openNodesFiltered retrieves specific nodes by name.

// When includeArchived is false (default), entities with _status:archived are excluded.

func (s store) openNodesFiltered(names []string, includeArchived bool) (KnowledgeGraph, error) {

	if len(names) == 0 {

		return KnowledgeGraph{}, nil

	}

	var matchingEntities []dbEntity

	var err error

	if includeArchived {

		err = s.db.Where("name IN ?", names).Find(&matchingEntities).Error

	} else {

		err = s.db.Raw(`

			SELECT e.* FROM db_entities e

			WHERE e.name IN ?

			  AND e.name NOT IN (

			    SELECT entity_name FROM db_observations WHERE content = '_status:archived'

			  )

		`, names).Scan(&matchingEntities).Error

	}

	if err != nil {

		return KnowledgeGraph{}, err

	}

	return s.buildSubGraph(matchingEntities)

}

func (s store) buildSubGraph(entities []dbEntity) (KnowledgeGraph, error) {

	if len(entities) == 0 {

		return KnowledgeGraph{}, nil

	}

	names := make([]string, 0, len(entities))

	for _, e := range entities {

		names = append(names, e.Name)

	}

	var dbObs []dbObservation

	if err := s.db.Where("entity_name IN ?", names).Find(&dbObs).Error; err != nil {

		return KnowledgeGraph{}, err

	}

	obsMap := make(map[string][]string)

	for _, obs := range dbObs {

		obsMap[obs.EntityName] = append(obsMap[obs.EntityName], obs.Content)

	}

	var resultEntities []Entity

	for _, e := range entities {

		resultEntities = append(resultEntities, Entity{

			Name: e.Name,

			EntityType: e.EntityType,

			Observations: obsMap[e.Name],
		})

	}

	var dbRels []dbRelation

	if err := s.db.Where("from_node IN ? AND to_node IN ?", names, names).Find(&dbRels).Error; err != nil {

		return KnowledgeGraph{}, err

	}

	var resultRels []Relation

	for _, r := range dbRels {

		resultRels = append(resultRels, Relation{

			From: r.FromEntity,

			To: r.ToEntity,

			RelationType: r.RelationType,
		})

	}

	return KnowledgeGraph{Entities: resultEntities, Relations: resultRels}, nil

}

// --- Domain API ---

// MemoryService exposes a clean data-layer API for the knowledge graph.

// It shares the same *gorm.DB as the internal structure.

type MemoryService struct {
	s store
}

// NewMemoryService creates a MemoryService using an already-opened *gorm.DB.

func NewMemoryService(db *gorm.DB) *MemoryService {

	return &MemoryService{s: store{db: db}}

}

// CreateEntities creates entities, skipping duplicates.

func (srv *MemoryService) CreateEntities(entities []Entity) ([]Entity, error) {

	return srv.s.createEntities(entities)

}

// CreateRelations creates relations, skipping duplicates.

func (srv *MemoryService) CreateRelations(relations []Relation) ([]Relation, error) {

	return srv.s.createRelations(relations)

}

// AddObservations appends observations to existing entities, skipping duplicates.

func (srv *MemoryService) AddObservations(obs []Observation) ([]Observation, error) {

	return srv.s.addObservations(obs)

}

// DeleteEntities removes entities and their associated relations/observations.

func (srv *MemoryService) DeleteEntities(names []string) error {

	return srv.s.deleteEntities(names)

}

// DeleteObservations removes specific observations.

func (srv *MemoryService) DeleteObservations(deletions []Observation) error {

	return srv.s.deleteObservations(deletions)

}

// DeleteRelations removes specific relations.

func (srv *MemoryService) DeleteRelations(relations []Relation) error {

	return srv.s.deleteRelations(relations)

}

// ReadGraph returns the entire knowledge graph.

func (srv *MemoryService) ReadGraph() (KnowledgeGraph, error) {

	return srv.s.loadGraph()

}

// SearchNodes searches entities by name, type, or observation content.

// Archived entities (_status:archived) are excluded by default.

func (srv *MemoryService) SearchNodes(query string) (KnowledgeGraph, error) {

	return srv.s.searchNodes(query)

}

// SearchNodesFiltered searches entities with optional inclusion of archived entities.

func (srv *MemoryService) SearchNodesFiltered(query string, includeArchived bool) (KnowledgeGraph, error) {

	return srv.s.searchNodesFiltered(query, includeArchived)

}

// OpenNodes retrieves specific nodes by name.

// Archived entities (_status:archived) are excluded by default.

func (srv *MemoryService) OpenNodes(names []string) (KnowledgeGraph, error) {

	return srv.s.openNodes(names)

}

// OpenNodesFiltered retrieves specific nodes by name with optional inclusion of archived entities.

func (srv *MemoryService) OpenNodesFiltered(names []string, includeArchived bool) (KnowledgeGraph, error) {

	return srv.s.openNodesFiltered(names, includeArchived)

}

// TraverseGraph performs a BFS from entity, following edges up to maxDepth hops.

// direction must be "outgoing", "incoming", or "both".

// relationType filters by edge type; empty string matches all types.

// The start entity is not included in the results.

func (srv *MemoryService) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]TraverseNode, error) {

	return srv.s.traverseGraph(entity, relationType, direction, maxDepth)

}

// ListEntities returns all entities whose entity_type matches entityType (case-insensitive).

// When entityType is empty, all entities are returned (equivalent to ReadGraph but without

// loading relations — use ReadGraph if you need edges too).

func (srv *MemoryService) ListEntities(entityType string) (KnowledgeGraph, error) {

	return srv.s.listEntities(entityType)

}

// ListRelations returns all relations, optionally filtered by relation_type and/or from_entity.

// Empty string parameters act as wildcards.

func (srv *MemoryService) ListRelations(relationType, fromEntity string) ([]Relation, error) {

	return srv.s.listRelations(relationType, fromEntity)

}

// EntityCount returns the total number of entities in the knowledge graph.

func (srv *MemoryService) EntityCount() (int64, error) {

	var count int64

	err := srv.s.db.Model(&dbEntity{}).Count(&count).Error

	return count, err

}

// EntityTypeCounts returns the count of entities grouped by entity_type.

func (srv *MemoryService) EntityTypeCounts() (map[string]int64, error) {

	type row struct {
		EntityType string

		Count int64
	}

	var rows []row

	if err := srv.s.db.Model(&dbEntity{}).
		Select("entity_type, COUNT(*) AS count").
		Group("entity_type").
		Scan(&rows).Error; err != nil {

		return nil, err

	}

	counts := make(map[string]int64, len(rows))

	for _, r := range rows {

		counts[r.EntityType] = r.Count

	}

	return counts, nil

}

// GetIntegrationMap returns all integration edges for service up to depth hops.

// depth is clamped to [1, 3].

func (srv *MemoryService) GetIntegrationMap(ctx context.Context, service string, depth int) (IntegrationMap, error) {

	return srv.s.getIntegrationMap(ctx, service, depth)

}

// FindPath finds the shortest undirected path between two entities using BFS.

// maxDepth is clamped to [1, 10]. Returns an empty slice when no path is found.

func (srv *MemoryService) FindPath(from, to string, maxDepth int) ([]PathEdge, error) {

	return srv.s.findPath(from, to, maxDepth)

}

// UpdateEntity renames an entity and/or changes its entity type atomically.

// Renaming cascades to all relations and observations that reference the old name.

// At least one of newName or newType must be non-empty.

func (srv *MemoryService) UpdateEntity(oldName, newName, newType string) error {

	return srv.s.updateEntity(oldName, newName, newType)

}

// OpenDB opens the database connection and runs auto-migration for all models.

// dbURL accepts: "sqlite://path.db", "postgres://...", a plain file path, or "" for in-memory SQLite.

func OpenDB(dbURL string) (*gorm.DB, error) {

	cfg := &gorm.Config{

		Logger: logger.Default.LogMode(logger.Silent),
	}

	var db *gorm.DB

	var err error

	switch {

	case strings.HasPrefix(dbURL, "postgres://"), strings.HasPrefix(dbURL, "postgresql://"):

		db, err = gorm.Open(postgres.Open(dbURL), cfg)

	case strings.HasPrefix(dbURL, "sqlite://"):

		path := strings.TrimPrefix(dbURL, "sqlite://")

		db, err = gorm.Open(sqlite.Open(path), cfg)

	case dbURL == "":

		// Each call gets a unique named in-memory database so tests never share state.

		name := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared", inMemoryCounter.Add(1))

		db, err = gorm.Open(sqlite.Open(name), cfg)

	default:

		db, err = gorm.Open(sqlite.Open(dbURL), cfg)

	}

	if err != nil {

		return nil, err

	}

	if err := db.AutoMigrate(&dbEntity{}, &dbRelation{}, &dbObservation{}, &dbDocContent{}); err != nil {

		return nil, err

	}

	return db, nil

}
