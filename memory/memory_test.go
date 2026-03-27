// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testCounter atomic.Int64

func newTestStore(t *testing.T) store {
	t.Helper()
	n := testCounter.Add(1)
	dsn := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", n)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&dbEntity{}, &dbRelation{}, &dbObservation{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return store{db: db}
}

func TestCreateEntities(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Create two entities
	_, result, err := s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{
			{Name: "service-a", EntityType: "Component", Observations: []string{"Go microservice", "uses gRPC"}},
			{Name: "service-b", EntityType: "API"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entities) != 2 {
		t.Fatalf("expected 2 created entities, got %d", len(result.Entities))
	}

	// Duplicate should be skipped
	_, result2, err := s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{
			{Name: "service-a", EntityType: "Component"},
			{Name: "service-c", EntityType: "Database"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error on duplicate: %v", err)
	}
	if len(result2.Entities) != 1 || result2.Entities[0].Name != "service-c" {
		t.Fatalf("expected only service-c to be created, got %v", result2.Entities)
	}
}

func TestCreateRelations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Seed entities
	s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{
			{Name: "frontend", EntityType: "Component"},
			{Name: "backend", EntityType: "Component"},
		},
	})

	_, result, err := s.CreateRelations(ctx, req, CreateRelationsArgs{
		Relations: []Relation{
			{From: "frontend", To: "backend", RelationType: "dependsOn"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Relations) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(result.Relations))
	}

	// Duplicate relation should be skipped
	_, result2, err := s.CreateRelations(ctx, req, CreateRelationsArgs{
		Relations: []Relation{
			{From: "frontend", To: "backend", RelationType: "dependsOn"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error on duplicate: %v", err)
	}
	if len(result2.Relations) != 0 {
		t.Fatalf("expected 0 new relations (duplicate), got %d", len(result2.Relations))
	}
}

func TestAddObservations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Seed entity
	s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{{Name: "db-primary", EntityType: "Database"}},
	})

	_, result, err := s.AddObservations(ctx, req, AddObservationsArgs{
		Observations: []Observation{
			{EntityName: "db-primary", Contents: []string{"PostgreSQL 15", "runs on port 5432"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Observations) != 1 || len(result.Observations[0].Contents) != 2 {
		t.Fatalf("expected 2 new observations, got %v", result.Observations)
	}

	// Adding duplicate observation should be skipped
	_, result2, err := s.AddObservations(ctx, req, AddObservationsArgs{
		Observations: []Observation{
			{EntityName: "db-primary", Contents: []string{"PostgreSQL 15", "new fact"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2.Observations) != 1 || len(result2.Observations[0].Contents) != 1 {
		t.Fatalf("expected only 'new fact' to be added, got %v", result2.Observations)
	}

	// Error on non-existent entity
	_, _, err = s.AddObservations(ctx, req, AddObservationsArgs{
		Observations: []Observation{
			{EntityName: "non-existent", Contents: []string{"data"}},
		},
	})
	if err == nil {
		t.Fatal("expected error for non-existent entity")
	}
}

func TestSearchNodes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Seed
	s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{
			{Name: "auth-service", EntityType: "Component", Observations: []string{"handles JWT tokens"}},
			{Name: "payment-api", EntityType: "API", Observations: []string{"Stripe integration"}},
		},
	})
	s.CreateRelations(ctx, req, CreateRelationsArgs{
		Relations: []Relation{{From: "auth-service", To: "payment-api", RelationType: "authenticates"}},
	})

	// Search by name
	_, graph, err := s.SearchNodes(ctx, req, SearchNodesArgs{Query: "auth"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Entities) != 1 || graph.Entities[0].Name != "auth-service" {
		t.Fatalf("expected auth-service, got %v", graph.Entities)
	}

	// Search by observation content
	_, graph2, err := s.SearchNodes(ctx, req, SearchNodesArgs{Query: "stripe"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph2.Entities) != 1 || graph2.Entities[0].Name != "payment-api" {
		t.Fatalf("expected payment-api, got %v", graph2.Entities)
	}
}

func TestDeleteEntitiesCascade(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Seed
	s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{
			{Name: "svc-a", EntityType: "Component", Observations: []string{"obs1"}},
			{Name: "svc-b", EntityType: "Component"},
		},
	})
	s.CreateRelations(ctx, req, CreateRelationsArgs{
		Relations: []Relation{{From: "svc-a", To: "svc-b", RelationType: "calls"}},
	})

	// Delete svc-a → should cascade observations + relations
	_, _, err := s.DeleteEntities(ctx, req, DeleteEntitiesArgs{EntityNames: []string{"svc-a"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read graph and verify
	_, graph, err := s.ReadGraph(ctx, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Entities) != 1 || graph.Entities[0].Name != "svc-b" {
		t.Fatalf("expected only svc-b remaining, got %v", graph.Entities)
	}
	if len(graph.Relations) != 0 {
		t.Fatalf("expected 0 relations after cascade delete, got %d", len(graph.Relations))
	}
}

func TestReadGraph(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Empty graph
	_, graph, err := s.ReadGraph(ctx, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Entities) != 0 || len(graph.Relations) != 0 {
		t.Fatalf("expected empty graph, got entities=%d relations=%d", len(graph.Entities), len(graph.Relations))
	}
}

func TestOpenNodes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{
			{Name: "node-1", EntityType: "Service"},
			{Name: "node-2", EntityType: "Service"},
			{Name: "node-3", EntityType: "Database"},
		},
	})

	_, graph, err := s.OpenNodes(ctx, req, OpenNodesArgs{Names: []string{"node-1", "node-3"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(graph.Entities))
	}
}

func TestDeleteRelations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	s.CreateEntities(ctx, req, CreateEntitiesArgs{
		Entities: []Entity{
			{Name: "a", EntityType: "X"},
			{Name: "b", EntityType: "Y"},
		},
	})
	s.CreateRelations(ctx, req, CreateRelationsArgs{
		Relations: []Relation{
			{From: "a", To: "b", RelationType: "uses"},
			{From: "b", To: "a", RelationType: "notifies"},
		},
	})

	_, _, err := s.DeleteRelations(ctx, req, DeleteRelationsArgs{
		Relations: []Relation{{From: "a", To: "b", RelationType: "uses"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, graph, _ := s.ReadGraph(ctx, req, nil)
	if len(graph.Relations) != 1 || graph.Relations[0].RelationType != "notifies" {
		t.Fatalf("expected only 'notifies' relation remaining, got %v", graph.Relations)
	}
}
