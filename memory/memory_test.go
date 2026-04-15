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

func TestSearchNodes_ExcludesArchivedByDefault(t *testing.T) {
	s := newTestService(t)

	_, err := s.CreateEntities([]Entity{
		{Name: "active-svc", EntityType: "service"},
		{Name: "gone-svc", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	// Mark gone-svc as archived.
	_, err = s.AddObservations([]Observation{
		{EntityName: "gone-svc", Contents: []string{"_status:archived"}},
	})
	if err != nil {
		t.Fatalf("AddObservations: %v", err)
	}

	// SearchNodes without includeArchived should hide gone-svc.
	graph, err := s.SearchNodes("svc")
	if err != nil {
		t.Fatalf("SearchNodes: %v", err)
	}
	for _, e := range graph.Entities {
		if e.Name == "gone-svc" {
			t.Error("SearchNodes should not return archived entity 'gone-svc' by default")
		}
	}
	if len(graph.Entities) != 1 || graph.Entities[0].Name != "active-svc" {
		t.Errorf("expected only active-svc, got: %v", graph.Entities)
	}

	// With includeArchived=true both entities should be returned.
	graphAll, err := s.SearchNodesFiltered("svc", true)
	if err != nil {
		t.Fatalf("SearchNodesFiltered: %v", err)
	}
	if len(graphAll.Entities) != 2 {
		t.Errorf("expected 2 entities with includeArchived=true, got %d", len(graphAll.Entities))
	}
}

func TestOpenNodes_ExcludesArchivedByDefault(t *testing.T) {
	s := newTestService(t)

	_, err := s.CreateEntities([]Entity{
		{Name: "live-svc", EntityType: "service"},
		{Name: "dead-svc", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities: %v", err)
	}

	_, err = s.AddObservations([]Observation{
		{EntityName: "dead-svc", Contents: []string{"_status:archived"}},
	})
	if err != nil {
		t.Fatalf("AddObservations: %v", err)
	}

	// OpenNodes with default should return only live-svc.
	graph, err := s.OpenNodes([]string{"live-svc", "dead-svc"})
	if err != nil {
		t.Fatalf("OpenNodes: %v", err)
	}
	if len(graph.Entities) != 1 || graph.Entities[0].Name != "live-svc" {
		t.Errorf("expected only live-svc, got: %v", graph.Entities)
	}

	// With includeArchived=true both are returned.
	graphAll, err := s.OpenNodesFiltered([]string{"live-svc", "dead-svc"}, true)
	if err != nil {
		t.Fatalf("OpenNodesFiltered: %v", err)
	}
	if len(graphAll.Entities) != 2 {
		t.Errorf("expected 2 entities with includeArchived=true, got %d", len(graphAll.Entities))
	}
}

func TestUpdateEntity_NewNameAlreadyExists(t *testing.T) {
	srv := newTestService(t)

	// Create two entities.
	_, err := srv.CreateEntities([]Entity{
		{Name: "alpha", EntityType: "service"},
		{Name: "beta", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities failed: %v", err)
	}

	// Renaming "alpha" to "beta" must be rejected — "beta" already exists.
	err = srv.UpdateEntity("alpha", "beta", "")
	if err == nil {
		t.Fatal("expected error when renaming to an existing entity name, got nil")
	}

	// Both entities must still exist with their original names.
	graph, err := srv.ReadGraph()
	if err != nil {
		t.Fatalf("ReadGraph failed: %v", err)
	}
	names := make(map[string]bool, len(graph.Entities))
	for _, e := range graph.Entities {
		names[e.Name] = true
	}
	if !names["alpha"] {
		t.Error("expected entity 'alpha' to still exist after rejected rename")
	}
	if !names["beta"] {
		t.Error("expected entity 'beta' to still exist after rejected rename")
	}
}
