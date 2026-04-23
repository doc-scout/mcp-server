// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

// Package tools — GraphAuditLogger wraps any GraphStore and emits a structured

// slog line for every mutation (create / add / delete). Read-only operations

// (ReadGraph, SearchNodes, OpenNodes, EntityCount) are delegated silently.

package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/doc-scout/mcp-server/memory"
)

// GraphAuditLogger is a GraphStore decorator that logs every mutation to slog
// and, when a store is provided, persists an AuditEvent row.

type GraphAuditLogger struct {
	inner   GraphStore
	agentFn func() string     // called per event; never nil
	store   memory.AuditStore // nil = no-op (in-memory deployments)
}

// NewGraphAuditLogger wraps inner with audit logging.
// agentFn is called on each write to resolve the current agent identity.
// store may be nil — audit persistence is skipped silently.
func NewGraphAuditLogger(inner GraphStore, agentFn func() string, store memory.AuditStore) *GraphAuditLogger {
	return &GraphAuditLogger{inner: inner, agentFn: agentFn, store: store}
}

func (a *GraphAuditLogger) writeAuditEvent(ctx context.Context, tool, operation string, targets []string, count int, outcome, errorMsg string) {
	if a.store == nil {
		return
	}
	event := memory.AuditEvent{
		Agent:     a.agentFn(),
		Tool:      tool,
		Operation: operation,
		Targets:   memory.MarshalTargets(targets),
		Count:     count,
		Outcome:   outcome,
		ErrorMsg:  errorMsg,
	}
	if err := a.store.Write(ctx, event); err != nil {
		slog.Warn("[graph:audit] failed to persist audit event", "tool", tool, "error", err)
	}
}

// ── Mutations ─────────────────────────────────────────────────────────────────

func (a *GraphAuditLogger) CreateEntities(entities []memory.Entity) ([]memory.Entity, error) {
	names := entityNames(entities)
	result, err := a.inner.CreateEntities(entities)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] create_entities failed", "names", names, "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] create_entities", "names", names, "count", len(result))
	}
	a.writeAuditEvent(context.Background(), "create_entities", "create", names, len(entities), outcome, errMsg)
	return result, err
}

func (a *GraphAuditLogger) CreateRelations(relations []memory.Relation) ([]memory.Relation, error) {
	result, err := a.inner.CreateRelations(relations)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] create_relations failed", "count", len(relations), "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] create_relations", "count", len(result))
	}
	a.writeAuditEvent(context.Background(), "create_relations", "create", []string{fmt.Sprintf("%d relations", len(relations))}, len(relations), outcome, errMsg)
	return result, err
}

func (a *GraphAuditLogger) AddObservations(observations []memory.Observation) ([]memory.Observation, error) {
	entities := observationEntityNames(observations)
	totalObs := countObservations(observations)
	result, err := a.inner.AddObservations(observations)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] add_observations failed", "entities", entities, "total_obs", totalObs, "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] add_observations", "entities", entities, "total_obs", totalObs)
	}
	a.writeAuditEvent(context.Background(), "add_observations", "add", entities, totalObs, outcome, errMsg)
	return result, err
}

func (a *GraphAuditLogger) DeleteEntities(entityNames []string) error {
	err := a.inner.DeleteEntities(entityNames)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] delete_entities failed", "names", entityNames, "count", len(entityNames), "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] delete_entities", "names", entityNames, "count", len(entityNames))
	}
	a.writeAuditEvent(context.Background(), "delete_entities", "delete", entityNames, len(entityNames), outcome, errMsg)
	return err
}

func (a *GraphAuditLogger) DeleteObservations(deletions []memory.Observation) error {
	entities := observationEntityNames(deletions)
	err := a.inner.DeleteObservations(deletions)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] delete_observations failed", "entities", entities, "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] delete_observations", "entities", entities, "count", len(deletions))
	}
	a.writeAuditEvent(context.Background(), "delete_observations", "delete", entities, len(deletions), outcome, errMsg)
	return err
}

func (a *GraphAuditLogger) DeleteRelations(relations []memory.Relation) error {
	err := a.inner.DeleteRelations(relations)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] delete_relations failed", "count", len(relations), "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] delete_relations", "count", len(relations))
	}
	a.writeAuditEvent(context.Background(), "delete_relations", "delete", []string{fmt.Sprintf("%d relations", len(relations))}, len(relations), outcome, errMsg)
	return err
}

func (a *GraphAuditLogger) UpdateEntity(oldName, newName, newType string) error {
	err := a.inner.UpdateEntity(oldName, newName, newType)
	outcome, errMsg := "ok", ""
	if err != nil {
		slog.Warn("[graph:audit] update_entity failed", "old_name", oldName, "new_name", newName, "new_type", newType, "error", err)
		outcome, errMsg = "error", err.Error()
	} else {
		slog.Info("[graph:audit] update_entity", "old_name", oldName, "new_name", newName, "new_type", newType)
	}
	a.writeAuditEvent(context.Background(), "update_entity", "update", []string{oldName}, 1, outcome, errMsg)
	return err
}

// ── Read-only pass-throughs ───────────────────────────────────────────────────

func (a *GraphAuditLogger) GetIntegrationMap(ctx context.Context, service string, depth int) (memory.IntegrationMap, error) {

	return a.inner.GetIntegrationMap(ctx, service, depth)

}

func (a *GraphAuditLogger) ListEntities(entityType string) (memory.KnowledgeGraph, error) {

	return a.inner.ListEntities(entityType)

}

func (a *GraphAuditLogger) ListRelations(relationType, fromEntity string) ([]memory.Relation, error) {

	return a.inner.ListRelations(relationType, fromEntity)

}

func (a *GraphAuditLogger) ReadGraph() (memory.KnowledgeGraph, error) {

	return a.inner.ReadGraph()

}

func (a *GraphAuditLogger) SearchNodes(query string) (memory.KnowledgeGraph, error) {

	return a.inner.SearchNodes(query)

}

func (a *GraphAuditLogger) SearchNodesFiltered(query string, includeArchived bool) (memory.KnowledgeGraph, error) {

	return a.inner.SearchNodesFiltered(query, includeArchived)

}

func (a *GraphAuditLogger) OpenNodes(names []string) (memory.KnowledgeGraph, error) {

	return a.inner.OpenNodes(names)

}

func (a *GraphAuditLogger) OpenNodesFiltered(names []string, includeArchived bool) (memory.KnowledgeGraph, error) {

	return a.inner.OpenNodesFiltered(names, includeArchived)

}

func (a *GraphAuditLogger) EntityCount() (int64, error) {

	return a.inner.EntityCount()

}

func (a *GraphAuditLogger) EntityTypeCounts() (map[string]int64, error) {

	return a.inner.EntityTypeCounts()

}

func (a *GraphAuditLogger) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]memory.TraverseNode, error) {

	return a.inner.TraverseGraph(entity, relationType, direction, maxDepth)

}

func (a *GraphAuditLogger) FindPath(from, to string, maxDepth int) ([]memory.PathEdge, error) {

	return a.inner.FindPath(from, to, maxDepth)

}

// ── Helpers ───────────────────────────────────────────────────────────────────

func entityNames(entities []memory.Entity) []string {

	names := make([]string, len(entities))

	for i, e := range entities {

		names[i] = e.Name

	}

	return names

}

func observationEntityNames(obs []memory.Observation) []string {

	seen := make(map[string]struct{}, len(obs))

	var names []string

	for _, o := range obs {

		if _, ok := seen[o.EntityName]; !ok {

			seen[o.EntityName] = struct{}{}

			names = append(names, o.EntityName)

		}

	}

	return names

}

func countObservations(obs []memory.Observation) int {

	total := 0

	for _, o := range obs {

		total += len(o.Contents)

	}

	return total

}
