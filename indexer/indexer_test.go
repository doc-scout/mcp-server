// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package indexer_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/leonancarvalho/docscout-mcp/indexer"
	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/leonancarvalho/docscout-mcp/scanner"
)

// --- Mock FileGetter ---

type mockFileGetter struct {
	files map[string]string // key: "repoName/path"
}

func (m *mockFileGetter) GetFileContent(ctx context.Context, repo, path string) (string, error) {
	key := repo + "/" + path
	if content, ok := m.files[key]; ok {
		return content, nil
	}
	return "", fmt.Errorf("not found: %s", key)
}

// --- Mock GraphWriter ---

type mockGraphWriter struct {
	entities  []memory.Entity
	relations []memory.Relation
}

func (m *mockGraphWriter) CreateEntities(entities []memory.Entity) ([]memory.Entity, error) {
	m.entities = append(m.entities, entities...)
	return entities, nil
}

func (m *mockGraphWriter) CreateRelations(relations []memory.Relation) ([]memory.Relation, error) {
	m.relations = append(m.relations, relations...)
	return relations, nil
}

func (m *mockGraphWriter) AddObservations(obs []memory.Observation) ([]memory.Observation, error) {
	for _, o := range obs {
		for i, e := range m.entities {
			if e.Name == o.EntityName {
				m.entities[i].Observations = append(m.entities[i].Observations, o.Contents...)
			}
		}
	}
	return obs, nil
}

func (m *mockGraphWriter) SearchNodes(query string) (memory.KnowledgeGraph, error) {
	var matched []memory.Entity
	for _, e := range m.entities {
		for _, obs := range e.Observations {
			if obs == query || containsStr(e.Observations, query) {
				matched = append(matched, e)
				break
			}
		}
	}
	return memory.KnowledgeGraph{Entities: matched}, nil
}

func (m *mockGraphWriter) EntityCount() (int64, error) {
	return int64(len(m.entities)), nil
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// --- Tests ---

func TestAutoIndexer_CreatesEntitiesFromCatalog(t *testing.T) {
	catalogYAML := `
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: payment-service
  description: Handles payment
spec:
  type: service
  lifecycle: production
  owner: team-payments
  dependsOn:
    - component:db
`
	fg := &mockFileGetter{
		files: map[string]string{
			"org/payment-service/catalog-info.yaml": catalogYAML,
		},
	}
	gw := &mockGraphWriter{}

	ai := indexer.New(fg, gw, nil)
	ai.Run(t.Context(), []scanner.RepoInfo{
		{
			Name:     "org/payment-service",
			FullName: "org/payment-service",
			Files: []scanner.FileEntry{
				{RepoName: "org/payment-service", Path: "catalog-info.yaml", Type: "catalog-info"},
			},
		},
	})

	if len(gw.entities) == 0 {
		t.Fatal("expected entities to be created")
	}

	found := false
	for _, e := range gw.entities {
		if e.Name == "payment-service" {
			found = true
			if e.EntityType != "service" {
				t.Errorf("expected entityType=service, got %s", e.EntityType)
			}
			// Must have auto-source observations
			if !containsStr(e.Observations, "_source:catalog-info") {
				t.Errorf("missing _source:catalog-info observation, got: %v", e.Observations)
			}
			if !containsStr(e.Observations, "_scan_repo:org/payment-service") {
				t.Errorf("missing _scan_repo observation, got: %v", e.Observations)
			}
		}
	}
	if !found {
		t.Errorf("payment-service entity not created; entities: %v", gw.entities)
	}

	// Verify depends_on relation was created
	depFound := false
	for _, r := range gw.relations {
		if r.From == "payment-service" && r.To == "component:db" && r.RelationType == "depends_on" {
			depFound = true
		}
	}
	if !depFound {
		t.Errorf("depends_on relation not created; relations: %v", gw.relations)
	}
}

func TestAutoIndexer_SkipsMalformedCatalog(t *testing.T) {
	fg := &mockFileGetter{
		files: map[string]string{
			"org/bad-svc/catalog-info.yaml": "this: is: not: valid: yaml: :::",
		},
	}
	gw := &mockGraphWriter{}
	ai := indexer.New(fg, gw, nil)

	// Should not panic or return error; just log and skip
	ai.Run(t.Context(), []scanner.RepoInfo{
		{
			Name: "org/bad-svc",
			Files: []scanner.FileEntry{
				{RepoName: "org/bad-svc", Path: "catalog-info.yaml", Type: "catalog-info"},
			},
		},
	})

	if len(gw.entities) != 0 {
		t.Errorf("expected no entities from malformed YAML, got %v", gw.entities)
	}
}

func TestAutoIndexer_SoftDeletesStaleEntities(t *testing.T) {
	// Pre-populate graph with an entity from a repo that won't be in the next scan
	gw := &mockGraphWriter{
		entities: []memory.Entity{
			{
				Name:       "old-service",
				EntityType: "service",
				Observations: []string{
					"_source:catalog-info",
					"_scan_repo:org/old-svc",
				},
			},
		},
	}
	fg := &mockFileGetter{files: map[string]string{}}
	ai := indexer.New(fg, gw, nil)

	// Run with an empty repo list (org/old-svc is gone)
	ai.Run(t.Context(), []scanner.RepoInfo{})

	// old-service should now have _status:archived
	archivedFound := false
	for _, e := range gw.entities {
		if e.Name == "old-service" {
			if containsStr(e.Observations, "_status:archived") {
				archivedFound = true
			}
		}
	}
	if !archivedFound {
		t.Errorf("expected _status:archived on stale entity; entities: %v", gw.entities)
	}
}

func TestAutoIndexer_PreservesManualEntities(t *testing.T) {
	// A manually-created entity (no _source:catalog-info) should not be overwritten
	gw := &mockGraphWriter{
		entities: []memory.Entity{
			{
				Name:         "payment-service",
				EntityType:   "service",
				Observations: []string{"manually added observation"},
				// No _source:catalog-info
			},
		},
	}
	catalogYAML := `
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: payment-service
spec:
  type: service
  lifecycle: production
`
	fg := &mockFileGetter{
		files: map[string]string{
			"org/payment-service/catalog-info.yaml": catalogYAML,
		},
	}
	ai := indexer.New(fg, gw, nil)
	ai.Run(t.Context(), []scanner.RepoInfo{
		{
			Name: "org/payment-service",
			Files: []scanner.FileEntry{
				{RepoName: "org/payment-service", Path: "catalog-info.yaml", Type: "catalog-info"},
			},
		},
	})

	// Manual observation must still be present
	for _, e := range gw.entities {
		if e.Name == "payment-service" {
			if !containsStr(e.Observations, "manually added observation") {
				t.Errorf("manual observation was removed; observations: %v", e.Observations)
			}
			return
		}
	}
	t.Error("payment-service entity not found after run")
}
