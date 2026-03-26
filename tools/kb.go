package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	EntityName string   `json:"entityName"`
	Contents   []string `json:"contents"`

	Observations []string `json:"observations,omitempty"` // Used for deletion operations
}

// KnowledgeGraph represents the complete graph structure.
type KnowledgeGraph struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}

// store provides persistence interface for knowledge base data.
type store interface {
	Read() ([]byte, error)
	Write(data []byte) error
}

type memoryStore struct {
	data []byte
}

func (ms *memoryStore) Read() ([]byte, error) {
	return ms.data, nil
}

func (ms *memoryStore) Write(data []byte) error {
	ms.data = data
	return nil
}

type fileStore struct {
	path string
}

func (fs *fileStore) Read() ([]byte, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read file %s: %w", fs.path, err)
	}
	return data, nil
}

func (fs *fileStore) Write(data []byte) error {
	if err := os.WriteFile(fs.path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write file %s: %w", fs.path, err)
	}
	return nil
}

type knowledgeBase struct {
	s store
}

type kbItem struct {
	Type string `json:"type"`

	// Entity fields
	Name         string   `json:"name,omitempty"`
	EntityType   string   `json:"entityType,omitempty"`
	Observations []string `json:"observations,omitempty"`

	// Relation fields
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	RelationType string `json:"relationType,omitempty"`
}

func (k knowledgeBase) loadGraph() (KnowledgeGraph, error) {
	data, err := k.s.Read()
	if err != nil {
		return KnowledgeGraph{}, fmt.Errorf("failed to read from store: %w", err)
	}

	if len(data) == 0 {
		return KnowledgeGraph{}, nil
	}

	var items []kbItem
	if err := json.Unmarshal(data, &items); err != nil {
		return KnowledgeGraph{}, fmt.Errorf("failed to unmarshal from store: %w", err)
	}

	graph := KnowledgeGraph{}

	for _, item := range items {
		switch item.Type {
		case "entity":
			graph.Entities = append(graph.Entities, Entity{
				Name:         item.Name,
				EntityType:   item.EntityType,
				Observations: item.Observations,
			})
		case "relation":
			graph.Relations = append(graph.Relations, Relation{
				From:         item.From,
				To:           item.To,
				RelationType: item.RelationType,
			})
		}
	}

	return graph, nil
}

func (k knowledgeBase) saveGraph(graph KnowledgeGraph) error {
	items := make([]kbItem, 0, len(graph.Entities)+len(graph.Relations))

	for _, entity := range graph.Entities {
		items = append(items, kbItem{
			Type:         "entity",
			Name:         entity.Name,
			EntityType:   entity.EntityType,
			Observations: entity.Observations,
		})
	}

	for _, relation := range graph.Relations {
		items = append(items, kbItem{
			Type:         "relation",
			From:         relation.From,
			To:           relation.To,
			RelationType: relation.RelationType,
		})
	}

	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}

	if err := k.s.Write(itemsJSON); err != nil {
		return fmt.Errorf("failed to write to store: %w", err)
	}
	return nil
}

func (k knowledgeBase) createEntities(entities []Entity) ([]Entity, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return nil, err
	}

	var newEntities []Entity
	for _, entity := range entities {
		if !slices.ContainsFunc(graph.Entities, func(e Entity) bool { return e.Name == entity.Name }) {
			newEntities = append(newEntities, entity)
			graph.Entities = append(graph.Entities, entity)
		}
	}

	if err := k.saveGraph(graph); err != nil {
		return nil, err
	}

	return newEntities, nil
}

func (k knowledgeBase) createRelations(relations []Relation) ([]Relation, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return nil, err
	}

	var newRelations []Relation
	for _, relation := range relations {
		exists := slices.ContainsFunc(graph.Relations, func(r Relation) bool {
			return r.From == relation.From &&
				r.To == relation.To &&
				r.RelationType == relation.RelationType
		})
		if !exists {
			newRelations = append(newRelations, relation)
			graph.Relations = append(graph.Relations, relation)
		}
	}

	if err := k.saveGraph(graph); err != nil {
		return nil, err
	}

	return newRelations, nil
}

func (k knowledgeBase) addObservations(observations []Observation) ([]Observation, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return nil, err
	}

	var results []Observation

	for _, obs := range observations {
		entityIndex := slices.IndexFunc(graph.Entities, func(e Entity) bool { return e.Name == obs.EntityName })
		if entityIndex == -1 {
			return nil, fmt.Errorf("entity with name %s not found", obs.EntityName)
		}

		var newObservations []string
		for _, content := range obs.Contents {
			if !slices.Contains(graph.Entities[entityIndex].Observations, content) {
				newObservations = append(newObservations, content)
				graph.Entities[entityIndex].Observations = append(graph.Entities[entityIndex].Observations, content)
			}
		}

		results = append(results, Observation{
			EntityName: obs.EntityName,
			Contents:   newObservations,
		})
	}

	if err := k.saveGraph(graph); err != nil {
		return nil, err
	}

	return results, nil
}

func (k knowledgeBase) deleteEntities(entityNames []string) error {
	graph, err := k.loadGraph()
	if err != nil {
		return err
	}

	entitiesToDelete := make(map[string]bool)
	for _, name := range entityNames {
		entitiesToDelete[name] = true
	}

	graph.Entities = slices.DeleteFunc(graph.Entities, func(entity Entity) bool {
		return entitiesToDelete[entity.Name]
	})

	graph.Relations = slices.DeleteFunc(graph.Relations, func(relation Relation) bool {
		return entitiesToDelete[relation.From] || entitiesToDelete[relation.To]
	})

	return k.saveGraph(graph)
}

func (k knowledgeBase) deleteObservations(deletions []Observation) error {
	graph, err := k.loadGraph()
	if err != nil {
		return err
	}

	for _, deletion := range deletions {
		entityIndex := slices.IndexFunc(graph.Entities, func(e Entity) bool {
			return e.Name == deletion.EntityName
		})
		if entityIndex == -1 {
			continue
		}

		observationsToDelete := make(map[string]bool)
		for _, observation := range deletion.Observations {
			observationsToDelete[observation] = true
		}

		graph.Entities[entityIndex].Observations = slices.DeleteFunc(graph.Entities[entityIndex].Observations, func(observation string) bool {
			return observationsToDelete[observation]
		})
	}

	return k.saveGraph(graph)
}

func (k knowledgeBase) deleteRelations(relations []Relation) error {
	graph, err := k.loadGraph()
	if err != nil {
		return err
	}

	graph.Relations = slices.DeleteFunc(graph.Relations, func(existingRelation Relation) bool {
		return slices.ContainsFunc(relations, func(relationToDelete Relation) bool {
			return existingRelation.From == relationToDelete.From &&
				existingRelation.To == relationToDelete.To &&
				existingRelation.RelationType == relationToDelete.RelationType
		})
	})
	return k.saveGraph(graph)
}

func (k knowledgeBase) searchNodes(query string) (KnowledgeGraph, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return KnowledgeGraph{}, err
	}

	queryLower := strings.ToLower(query)
	var filteredEntities []Entity

	for _, entity := range graph.Entities {
		if strings.Contains(strings.ToLower(entity.Name), queryLower) ||
			strings.Contains(strings.ToLower(entity.EntityType), queryLower) {
			filteredEntities = append(filteredEntities, entity)
			continue
		}

		for _, observation := range entity.Observations {
			if strings.Contains(strings.ToLower(observation), queryLower) {
				filteredEntities = append(filteredEntities, entity)
				break
			}
		}
	}

	filteredEntityNames := make(map[string]bool)
	for _, entity := range filteredEntities {
		filteredEntityNames[entity.Name] = true
	}

	var filteredRelations []Relation
	for _, relation := range graph.Relations {
		if filteredEntityNames[relation.From] && filteredEntityNames[relation.To] {
			filteredRelations = append(filteredRelations, relation)
		}
	}

	return KnowledgeGraph{
		Entities:  filteredEntities,
		Relations: filteredRelations,
	}, nil
}

func (k knowledgeBase) openNodes(names []string) (KnowledgeGraph, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return KnowledgeGraph{}, err
	}

	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}

	var filteredEntities []Entity
	for _, entity := range graph.Entities {
		if nameSet[entity.Name] {
			filteredEntities = append(filteredEntities, entity)
		}
	}

	filteredEntityNames := make(map[string]bool)
	for _, entity := range filteredEntities {
		filteredEntityNames[entity.Name] = true
	}

	var filteredRelations []Relation
	for _, relation := range graph.Relations {
		if filteredEntityNames[relation.From] && filteredEntityNames[relation.To] {
			filteredRelations = append(filteredRelations, relation)
		}
	}

	return KnowledgeGraph{
		Entities:  filteredEntities,
		Relations: filteredRelations,
	}, nil
}

// --- Handler Args and Methods

type CreateEntitiesArgs struct {
	Entities []Entity `json:"entities" jsonschema:"entities to create"`
}
type CreateEntitiesResult struct {
	Entities []Entity `json:"entities"`
}
func (k knowledgeBase) CreateEntities(ctx context.Context, req *mcp.CallToolRequest, args CreateEntitiesArgs) (*mcp.CallToolResult, CreateEntitiesResult, error) {
	entities, err := k.createEntities(args.Entities)
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
func (k knowledgeBase) CreateRelations(ctx context.Context, req *mcp.CallToolRequest, args CreateRelationsArgs) (*mcp.CallToolResult, CreateRelationsResult, error) {
	relations, err := k.createRelations(args.Relations)
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
func (k knowledgeBase) AddObservations(ctx context.Context, req *mcp.CallToolRequest, args AddObservationsArgs) (*mcp.CallToolResult, AddObservationsResult, error) {
	observations, err := k.addObservations(args.Observations)
	if err != nil {
		return nil, AddObservationsResult{}, err
	}
	return nil, AddObservationsResult{Observations: observations}, nil
}

type DeleteEntitiesArgs struct {
	EntityNames []string `json:"entityNames" jsonschema:"entities to delete"`
}
func (k knowledgeBase) DeleteEntities(ctx context.Context, req *mcp.CallToolRequest, args DeleteEntitiesArgs) (*mcp.CallToolResult, any, error) {
	err := k.deleteEntities(args.EntityNames)
	return nil, nil, err
}

type DeleteObservationsArgs struct {
	Deletions []Observation `json:"deletions" jsonschema:"observations to delete"`
}
func (k knowledgeBase) DeleteObservations(ctx context.Context, req *mcp.CallToolRequest, args DeleteObservationsArgs) (*mcp.CallToolResult, any, error) {
	err := k.deleteObservations(args.Deletions)
	return nil, nil, err
}

type DeleteRelationsArgs struct {
	Relations []Relation `json:"relations" jsonschema:"relations to delete"`
}
func (k knowledgeBase) DeleteRelations(ctx context.Context, req *mcp.CallToolRequest, args DeleteRelationsArgs) (*mcp.CallToolResult, struct{}, error) {
	err := k.deleteRelations(args.Relations)
	return nil, struct{}{}, err
}

func (k knowledgeBase) ReadGraph(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, KnowledgeGraph, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return nil, KnowledgeGraph{}, err
	}
	return nil, graph, nil
}

type SearchNodesArgs struct {
	Query string `json:"query" jsonschema:"query string"`
}
func (k knowledgeBase) SearchNodes(ctx context.Context, req *mcp.CallToolRequest, args SearchNodesArgs) (*mcp.CallToolResult, KnowledgeGraph, error) {
	graph, err := k.searchNodes(args.Query)
	if err != nil {
		return nil, KnowledgeGraph{}, err
	}
	return nil, graph, nil
}

type OpenNodesArgs struct {
	Names []string `json:"names" jsonschema:"names of nodes to open"`
}
func (k knowledgeBase) OpenNodes(ctx context.Context, req *mcp.CallToolRequest, args OpenNodesArgs) (*mcp.CallToolResult, KnowledgeGraph, error) {
	graph, err := k.openNodes(args.Names)
	if err != nil {
		return nil, KnowledgeGraph{}, err
	}
	return nil, graph, nil
}

// RegisterMemory adds the memory capabilities to the MCP server.
func RegisterMemory(s *mcp.Server, memoryFilePath string) {
	var kbStore store
	kbStore = &memoryStore{}
	if memoryFilePath != "" {
		kbStore = &fileStore{path: memoryFilePath}
	}
	kb := knowledgeBase{s: kbStore}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_entities",
		Description: "Create multiple new entities in the knowledge graph",
	}, kb.CreateEntities)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_relations",
		Description: "Create multiple new relations between entities",
	}, kb.CreateRelations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_observations",
		Description: "Add new observations to existing entities",
	}, kb.AddObservations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_entities",
		Description: "Remove entities and their relations",
	}, kb.DeleteEntities)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_observations",
		Description: "Remove specific observations from entities",
	}, kb.DeleteObservations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_relations",
		Description: "Remove specific relations from the graph",
	}, kb.DeleteRelations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "read_graph",
		Description: "Read the entire knowledge graph",
	}, kb.ReadGraph)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_nodes",
		Description: "Search for nodes based on query",
	}, kb.SearchNodes)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "open_nodes",
		Description: "Retrieve specific nodes by name",
	}, kb.OpenNodes)
}
