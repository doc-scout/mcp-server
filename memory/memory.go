// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// --- API Structs (JSON + MCP Schema) ---

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

// --- GORM DB Models ---

type dbEntity struct {
	Name       string `gorm:"primaryKey"`
	EntityType string `gorm:"index"`
}

type dbRelation struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	FromEntity   string `gorm:"index;column:from_node"`
	ToEntity     string `gorm:"index;column:to_node"`
	RelationType string `gorm:"index"`
}

type dbObservation struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	EntityName string `gorm:"index;column:entity_name"`
	Content    string
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
			Name:         e.Name,
			EntityType:   e.EntityType,
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
			From:         r.FromEntity,
			To:           r.ToEntity,
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

func (s store) searchNodes(query string) (KnowledgeGraph, error) {
	queryPattern := "%" + strings.ToLower(query) + "%"

	var matchingEntities []dbEntity
	if err := s.db.Raw(`
		SELECT DISTINCT e.* FROM db_entities e
		LEFT JOIN db_observations o ON e.name = o.entity_name
		WHERE LOWER(e.name) LIKE ? OR LOWER(e.entity_type) LIKE ? OR LOWER(o.content) LIKE ?
	`, queryPattern, queryPattern, queryPattern).Scan(&matchingEntities).Error; err != nil {
		return KnowledgeGraph{}, err
	}

	return s.buildSubGraph(matchingEntities)
}

func (s store) openNodes(names []string) (KnowledgeGraph, error) {
	if len(names) == 0 {
		return KnowledgeGraph{}, nil
	}

	var matchingEntities []dbEntity
	if err := s.db.Where("name IN ?", names).Find(&matchingEntities).Error; err != nil {
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
			Name:         e.Name,
			EntityType:   e.EntityType,
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
			From:         r.FromEntity,
			To:           r.ToEntity,
			RelationType: r.RelationType,
		})
	}

	return KnowledgeGraph{Entities: resultEntities, Relations: resultRels}, nil
}

// --- MCP Tool Handler Args ---

type CreateEntitiesArgs struct {
	Entities []Entity `json:"entities" jsonschema:"entities to create"`
}
type CreateEntitiesResult struct {
	Entities []Entity `json:"entities"`
}

func (s store) CreateEntities(ctx context.Context, req *mcp.CallToolRequest, args CreateEntitiesArgs) (*mcp.CallToolResult, CreateEntitiesResult, error) {
	entities, err := s.createEntities(args.Entities)
	if err != nil {
		return nil, CreateEntitiesResult{}, err
	}
	return nil, CreateEntitiesResult{Entities: entities}, nil
}

type CreateRelationsArgs struct {
	Relations []Relation `json:"relations" jsonschema:"relations to create"`
}
type CreateRelationsResult struct {
	Relations []Relation `json:"relations"`
}

func (s store) CreateRelations(ctx context.Context, req *mcp.CallToolRequest, args CreateRelationsArgs) (*mcp.CallToolResult, CreateRelationsResult, error) {
	relations, err := s.createRelations(args.Relations)
	if err != nil {
		return nil, CreateRelationsResult{}, err
	}
	return nil, CreateRelationsResult{Relations: relations}, nil
}

type AddObservationsArgs struct {
	Observations []Observation `json:"observations" jsonschema:"observations to add"`
}
type AddObservationsResult struct {
	Observations []Observation `json:"observations"`
}

func (s store) AddObservations(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {
	observations, err := s.addObservations(args.Observations)
	if err != nil {
		return nil, AddObservationsResult{}, err
	}
	return nil, AddObservationsResult{Observations: observations}, nil
}

type DeleteEntitiesArgs struct {
	EntityNames []string `json:"entityNames" jsonschema:"entities to delete"`
}

func (s store) DeleteEntities(ctx context.Context, req *mcp.CallToolRequest, args DeleteEntitiesArgs) (*mcp.CallToolResult, any, error) {
	err := s.deleteEntities(args.EntityNames)
	return nil, nil, err
}

type DeleteObservationsArgs struct {
	Deletions []Observation `json:"deletions" jsonschema:"observations to delete"`
}

func (s store) DeleteObservations(ctx context.Context, req *mcp.CallToolRequest, args DeleteObservationsArgs) (*mcp.CallToolResult, any, error) {
	err := s.deleteObservations(args.Deletions)
	return nil, nil, err
}

type DeleteRelationsArgs struct {
	Relations []Relation `json:"relations" jsonschema:"relations to delete"`
}

func (s store) DeleteRelations(ctx context.Context, req *mcp.CallToolRequest, args DeleteRelationsArgs) (*mcp.CallToolResult, struct{}, error) {
	err := s.deleteRelations(args.Relations)
	return nil, struct{}{}, err
}

func (s store) ReadGraph(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, KnowledgeGraph, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, KnowledgeGraph{}, err
	}
	return nil, graph, nil
}

type SearchNodesArgs struct {
	Query string `json:"query" jsonschema:"query string"`
}

func (s store) SearchNodes(ctx context.Context, req *mcp.CallToolRequest, args SearchNodesArgs) (*mcp.CallToolResult, KnowledgeGraph, error) {
	graph, err := s.searchNodes(args.Query)
	if err != nil {
		return nil, KnowledgeGraph{}, err
	}
	return nil, graph, nil
}

type OpenNodesArgs struct {
	Names []string `json:"names" jsonschema:"names of nodes to open"`
}

func (s store) OpenNodes(ctx context.Context, req *mcp.CallToolRequest, args OpenNodesArgs) (*mcp.CallToolResult, KnowledgeGraph, error) {
	graph, err := s.openNodes(args.Names)
	if err != nil {
		return nil, KnowledgeGraph{}, err
	}
	return nil, graph, nil
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
		db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), cfg)
	default:
		db, err = gorm.Open(sqlite.Open(dbURL), cfg)
	}

	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&dbEntity{}, &dbRelation{}, &dbObservation{}); err != nil {
		return nil, err
	}
	return db, nil
}

// Register adds the knowledge graph memory tools to the MCP server.
// db must be obtained via OpenDB (already migrated).
func Register(s *mcp.Server, db *gorm.DB) {
	mem := store{db: db}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_entities",
		Description: "Create multiple new entities in the knowledge graph",
	}, mem.CreateEntities)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_relations",
		Description: "Create multiple new relations between entities",
	}, mem.CreateRelations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_observations",
		Description: "Add new observations to existing entities",
	}, mem.AddObservations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_entities",
		Description: "Remove entities and their relations",
	}, mem.DeleteEntities)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_observations",
		Description: "Remove specific observations from entities",
	}, mem.DeleteObservations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_relations",
		Description: "Remove specific relations from the graph",
	}, mem.DeleteRelations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "read_graph",
		Description: "Read the entire knowledge graph",
	}, mem.ReadGraph)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_nodes",
		Description: "Search for nodes based on query",
	}, mem.SearchNodes)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "open_nodes",
		Description: "Retrieve specific nodes by name",
	}, mem.OpenNodes)

	log.Printf("[memory] Knowledge graph initialized")
}

// AutoWriter exposes a clean data-layer API for the auto-indexer.
// It shares the same *gorm.DB as the MCP tool store.
type AutoWriter struct {
	s store
}

// NewAutoWriter creates an AutoWriter using an already-opened *gorm.DB.
func NewAutoWriter(db *gorm.DB) *AutoWriter {
	return &AutoWriter{s: store{db: db}}
}

// CreateEntities creates entities, skipping duplicates.
func (w *AutoWriter) CreateEntities(entities []Entity) ([]Entity, error) {
	return w.s.createEntities(entities)
}

// CreateRelations creates relations, skipping duplicates.
func (w *AutoWriter) CreateRelations(relations []Relation) ([]Relation, error) {
	return w.s.createRelations(relations)
}

// AddObservations appends observations to existing entities, skipping duplicates.
func (w *AutoWriter) AddObservations(obs []Observation) ([]Observation, error) {
	return w.s.addObservations(obs)
}

// SearchNodes searches entities by name, type, or observation content.
func (w *AutoWriter) SearchNodes(query string) (KnowledgeGraph, error) {
	return w.s.searchNodes(query)
}

// EntityCount returns the total number of entities in the knowledge graph.
func (w *AutoWriter) EntityCount() (int64, error) {
	var count int64
	err := w.s.db.Model(&dbEntity{}).Count(&count).Error
	return count, err
}
