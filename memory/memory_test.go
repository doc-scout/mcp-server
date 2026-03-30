// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"fmt"
	"sync/atomic"
	"testing"
)

var testCounter atomic.Int64

func newTestService(t *testing.T) *MemoryService {
	t.Helper()
	n := testCounter.Add(1)
	dsn := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", n)
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	return NewMemoryService(db)
}

func TestCreateEntities(t *testing.T) {
	s := newTestService(t)

	// Create two entities
	result, err := s.CreateEntities([]Entity{
		{Name: "service-a", EntityType: "Component", Observations: []string{"Go microservice", "uses gRPC"}},
		{Name: "service-b", EntityType: "API"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 created entities, got %d", len(result))
	}

	// Duplicate should be skipped
	result2, err := s.CreateEntities([]Entity{
		{Name: "service-a", EntityType: "Component"},
		{Name: "service-c", EntityType: "Database"},
	})
	if err != nil {
		t.Fatalf("unexpected error on duplicate: %v", err)
	}
	if len(result2) != 1 || result2[0].Name != "service-c" {
		t.Fatalf("expected only service-c to be created, got %v", result2)
	}
}

func TestCreateRelations(t *testing.T) {
	s := newTestService(t)

	// Seed entities
	s.CreateEntities([]Entity{
		{Name: "frontend", EntityType: "Component"},
		{Name: "backend", EntityType: "Component"},
	})

	result, err := s.CreateRelations([]Relation{
		{From: "frontend", To: "backend", RelationType: "dependsOn"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(result))
	}

	// Duplicate relation should be skipped
	result2, err := s.CreateRelations([]Relation{
		{From: "frontend", To: "backend", RelationType: "dependsOn"},
	})
	if err != nil {
		t.Fatalf("unexpected error on duplicate: %v", err)
	}
	if len(result2) != 0 {
		t.Fatalf("expected 0 new relations (duplicate), got %d", len(result2))
	}
}

func TestAddObservations(t *testing.T) {
	s := newTestService(t)

	// Seed entity
	s.CreateEntities([]Entity{{Name: "db-primary", EntityType: "Database"}})

	result, err := s.AddObservations([]Observation{
		{EntityName: "db-primary", Contents: []string{"PostgreSQL 15", "runs on port 5432"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || len(result[0].Contents) != 2 {
		t.Fatalf("expected 2 new observations, got %v", result)
	}

	// Adding duplicate observation should be skipped
	result2, err := s.AddObservations([]Observation{
		{EntityName: "db-primary", Contents: []string{"PostgreSQL 15", "new fact"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2) != 1 || len(result2[0].Contents) != 1 {
		t.Fatalf("expected only 'new fact' to be added, got %v", result2)
	}

	// Error on non-existent entity
	_, err = s.AddObservations([]Observation{
		{EntityName: "non-existent", Contents: []string{"data"}},
	})
	if err == nil {
		t.Fatal("expected error for non-existent entity")
	}
}

func TestSearchNodes(t *testing.T) {
	s := newTestService(t)

	// Seed
	s.CreateEntities([]Entity{
		{Name: "auth-service", EntityType: "Component", Observations: []string{"handles JWT tokens"}},
		{Name: "payment-api", EntityType: "API", Observations: []string{"Stripe integration"}},
	})
	s.CreateRelations([]Relation{{From: "auth-service", To: "payment-api", RelationType: "authenticates"}})

	// Search by name
	graph, err := s.SearchNodes("auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Entities) != 1 || graph.Entities[0].Name != "auth-service" {
		t.Fatalf("expected auth-service, got %v", graph.Entities)
	}

	// Search by observation content
	graph2, err := s.SearchNodes("stripe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph2.Entities) != 1 || graph2.Entities[0].Name != "payment-api" {
		t.Fatalf("expected payment-api, got %v", graph2.Entities)
	}
}

func TestDeleteEntitiesCascade(t *testing.T) {
	s := newTestService(t)

	// Seed
	s.CreateEntities([]Entity{
		{Name: "svc-a", EntityType: "Component", Observations: []string{"obs1"}},
		{Name: "svc-b", EntityType: "Component"},
	})
	s.CreateRelations([]Relation{{From: "svc-a", To: "svc-b", RelationType: "calls"}})

	// Delete svc-a → should cascade observations + relations
	err := s.DeleteEntities([]string{"svc-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read graph and verify
	graph, err := s.ReadGraph()
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
	s := newTestService(t)

	// Empty graph
	graph, err := s.ReadGraph()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Entities) != 0 || len(graph.Relations) != 0 {
		t.Fatalf("expected empty graph, got entities=%d relations=%d", len(graph.Entities), len(graph.Relations))
	}
}

func TestOpenNodes(t *testing.T) {
	s := newTestService(t)

	s.CreateEntities([]Entity{
		{Name: "node-1", EntityType: "Service"},
		{Name: "node-2", EntityType: "Service"},
		{Name: "node-3", EntityType: "Database"},
	})

	graph, err := s.OpenNodes([]string{"node-1", "node-3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(graph.Entities))
	}
}

func TestDeleteRelations(t *testing.T) {
	s := newTestService(t)

	s.CreateEntities([]Entity{
		{Name: "a", EntityType: "X"},
		{Name: "b", EntityType: "Y"},
	})
	s.CreateRelations([]Relation{
		{From: "a", To: "b", RelationType: "uses"},
		{From: "b", To: "a", RelationType: "notifies"},
	})

	err := s.DeleteRelations([]Relation{{From: "a", To: "b", RelationType: "uses"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	graph, _ := s.ReadGraph()
	if len(graph.Relations) != 1 || graph.Relations[0].RelationType != "notifies" {
		t.Fatalf("expected only 'notifies' relation remaining, got %v", graph.Relations)
	}
}

func TestOpenDB_InMemory(t *testing.T) {
	db, err := OpenDB("")
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestMemoryService_EntityCount(t *testing.T) {
	db, err := OpenDB(fmt.Sprintf("file:autowriter_%d?mode=memory&cache=shared", testCounter.Add(1)))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	w := NewMemoryService(db)

	count, err := w.EntityCount()
	if err != nil {
		t.Fatalf("EntityCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 entities, got %d", count)
	}

	_, err = w.CreateEntities([]Entity{
		{Name: "svc-x", EntityType: "service"},
		{Name: "svc-y", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	count, err = w.EntityCount()
	if err != nil {
		t.Fatalf("EntityCount after create: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 entities, got %d", count)
	}
}
