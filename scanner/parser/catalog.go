// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParsedCatalog holds the data extracted from a Backstage catalog-info.yaml.
type ParsedCatalog struct {
	EntityName   string
	EntityType   string
	Observations []string
	Relations    []ParsedRelation
}

// ParsedRelation is a directed edge extracted from catalog-info.yaml.
type ParsedRelation struct {
	From         string
	To           string
	RelationType string
}

type backstageCatalog struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description"`
		Tags        []string `yaml:"tags"`
	} `yaml:"metadata"`
	Spec struct {
		Type         string   `yaml:"type"`
		Lifecycle    string   `yaml:"lifecycle"`
		Owner        string   `yaml:"owner"`
		System       string   `yaml:"system"`
		DependsOn    []string `yaml:"dependsOn"`
		ProvidesApis []string `yaml:"providesApis"`
		ConsumesApis []string `yaml:"consumesApis"`
	} `yaml:"spec"`
}

func kindToEntityType(kind, specType string) string {
	switch kind {
	case "API":
		return "api"
	case "System":
		return "system"
	case "Resource":
		return "resource"
	case "Group":
		return "team"
	case "Component":
		if specType != "" {
			return specType
		}
		return "component"
	default:
		return "component"
	}
}

// ParseCatalog parses raw catalog-info.yaml bytes into a ParsedCatalog.
// Returns an error only for YAML parse failures or missing metadata.name.
// Missing optional fields are silently skipped.
func ParseCatalog(data []byte) (ParsedCatalog, error) {
	var raw backstageCatalog
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ParsedCatalog{}, fmt.Errorf("catalog-info.yaml parse error: %w", err)
	}
	if raw.Metadata.Name == "" {
		return ParsedCatalog{}, fmt.Errorf("catalog-info.yaml: missing metadata.name")
	}

	name := raw.Metadata.Name
	entityType := kindToEntityType(raw.Kind, raw.Spec.Type)

	var obs []string
	if raw.Spec.Lifecycle != "" {
		obs = append(obs, "lifecycle:"+raw.Spec.Lifecycle)
	}
	if raw.Metadata.Description != "" {
		obs = append(obs, "description:"+raw.Metadata.Description)
	}
	for _, tag := range raw.Metadata.Tags {
		if tag != "" {
			obs = append(obs, "tag:"+tag)
		}
	}

	var rels []ParsedRelation
	if raw.Spec.Owner != "" {
		rels = append(rels, ParsedRelation{From: name, To: raw.Spec.Owner, RelationType: "owned_by"})
	}
	if raw.Spec.System != "" {
		rels = append(rels, ParsedRelation{From: name, To: raw.Spec.System, RelationType: "part_of"})
	}
	for _, dep := range raw.Spec.DependsOn {
		rels = append(rels, ParsedRelation{From: name, To: dep, RelationType: "depends_on"})
	}
	for _, api := range raw.Spec.ProvidesApis {
		rels = append(rels, ParsedRelation{From: name, To: api, RelationType: "provides_api"})
	}
	for _, api := range raw.Spec.ConsumesApis {
		rels = append(rels, ParsedRelation{From: name, To: api, RelationType: "consumes_api"})
	}

	return ParsedCatalog{
		EntityName:   name,
		EntityType:   entityType,
		Observations: obs,
		Relations:    rels,
	}, nil
}
