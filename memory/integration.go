// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"strings"
)

// IntegrationEdge is a single integration relationship entry in an IntegrationMap.
type IntegrationEdge struct {
	Target     string `json:"target"`
	Schema     string `json:"schema,omitempty"`      // event-topic schema name
	Version    string `json:"version,omitempty"`     // api version
	Paths      int    `json:"paths,omitempty"`       // api path count
	Confidence string `json:"confidence"`            // "authoritative" | "inferred"
	SourceRepo string `json:"source_repo,omitempty"` // originating repo
}

// IntegrationMap aggregates all integration relationships for a service.
type IntegrationMap struct {
	Service      string            `json:"service"`
	Publishes    []IntegrationEdge `json:"publishes"`
	Subscribes   []IntegrationEdge `json:"subscribes"`
	ExposesAPI   []IntegrationEdge `json:"exposes_api"`
	ProvidesGRPC []IntegrationEdge `json:"provides_grpc"`
	GRPCDeps     []IntegrationEdge `json:"grpc_deps"`
	Calls        []IntegrationEdge `json:"calls"`
	Coverage     string            `json:"graph_coverage"` // "full" | "partial" | "inferred" | "none"
}

// authoritativeSources are integration sources whose data comes from explicit contracts.
var authoritativeSources = map[string]bool{
	"asyncapi": true,
	"proto":    true,
	"openapi":  true,
}

// getIntegrationMap queries the graph for all integration edges of the given service
// up to the specified depth.
func (s store) getIntegrationMap(_ context.Context, service string, depth int) (IntegrationMap, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	result := IntegrationMap{Service: service}

	// Load the service entity's observations to determine integration sources.
	var obs []dbObservation
	if err := s.db.Where("entity_name = ?", service).Find(&obs).Error; err != nil {
		return result, err
	}

	integrationSources := make(map[string]bool)
	for _, o := range obs {
		if src, ok := strings.CutPrefix(o.Content, "_integration_source:"); ok {
			integrationSources[src] = true
		}
	}

	// confidence returns the confidence level for an edge based on the source parser.
	confidence := func(source string) string {
		if authoritativeSources[source] {
			return "authoritative"
		}
		return "inferred"
	}

	// Determine overall coverage from sources on the service entity.
	hasAuthoritative := false
	hasInferred := false
	for src := range integrationSources {
		if authoritativeSources[src] {
			hasAuthoritative = true
		} else {
			hasInferred = true
		}
	}

	// Query each integration relation type.
	integrationRelTypes := []struct {
		relType  string
		assignTo func([]IntegrationEdge)
		source   string
	}{
		{"publishes_event", func(e []IntegrationEdge) { result.Publishes = e }, "asyncapi"},
		{"subscribes_event", func(e []IntegrationEdge) { result.Subscribes = e }, "asyncapi"},
		{"exposes_api", func(e []IntegrationEdge) { result.ExposesAPI = e }, "openapi"},
		{"provides_grpc", func(e []IntegrationEdge) { result.ProvidesGRPC = e }, "proto"},
		{"depends_on_grpc", func(e []IntegrationEdge) { result.GRPCDeps = e }, "proto"},
		{"calls_service", func(e []IntegrationEdge) { result.Calls = e }, "k8s-env"},
	}

	hasAnyRelations := false
	for _, rt := range integrationRelTypes {
		var rels []dbRelation
		if err := s.db.Where("from_node = ? AND relation_type = ?", service, rt.relType).Find(&rels).Error; err != nil {
			return result, err
		}
		if len(rels) == 0 {
			continue
		}
		hasAnyRelations = true
		conf := confidence(rt.source)
		edges := make([]IntegrationEdge, 0, len(rels))
		for _, r := range rels {
			edges = append(edges, IntegrationEdge{
				Target:     r.ToEntity,
				Confidence: conf,
			})
		}
		rt.assignTo(edges)
	}

	// Compute graph_coverage.
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
