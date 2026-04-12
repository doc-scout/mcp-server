// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// openAPIDoc is the minimal shape we need from OpenAPI / Swagger documents.
type openAPIDoc struct {
	Info struct {
		Title   string `yaml:"title"   json:"title"`
		Version string `yaml:"version" json:"version"`
	} `yaml:"info" json:"info"`
	Servers []struct {
		URL string `yaml:"url" json:"url"`
	} `yaml:"servers" json:"servers"`
	// OpenAPI 3.x uses "paths"; Swagger 2.x also uses "paths".
	Paths map[string]interface{} `yaml:"paths" json:"paths"`
}

// openAPIParser implements FileParser for OpenAPI/Swagger documents.
type openAPIParser struct{}

// OpenAPIParser returns the FileParser for openapi.yaml/json and swagger.json/yaml files.
func OpenAPIParser() FileParser { return &openAPIParser{} }

func (*openAPIParser) FileType() string { return "openapi" }
func (*openAPIParser) Filenames() []string {
	return []string{"openapi.yaml", "openapi.json", "swagger.json", "swagger.yaml"}
}

func (p *openAPIParser) Parse(data []byte) (ParsedFile, error) {
	var doc openAPIDoc
	// Try JSON first (swagger.json), fall back to YAML.
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(data, &doc); err != nil {
			return ParsedFile{}, fmt.Errorf("openapi: json parse error: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedFile{}, fmt.Errorf("openapi: yaml parse error: %w", err)
		}
	}

	title := strings.TrimSpace(doc.Info.Title)
	if title == "" {
		return ParsedFile{}, fmt.Errorf("openapi: missing info.title")
	}

	obs := []string{"_integration_source:openapi"}
	if doc.Info.Version != "" {
		obs = append(obs, "version:"+doc.Info.Version)
	}
	if len(doc.Paths) > 0 {
		obs = append(obs, fmt.Sprintf("paths:%d", len(doc.Paths)))
	}
	for _, srv := range doc.Servers {
		if srv.URL != "" {
			obs = append(obs, "server_url:"+srv.URL)
		}
	}

	return ParsedFile{
		EntityName:   title,
		EntityType:   "api",
		Observations: obs,
		Relations: []ParsedRelation{
			// From is empty: filled by indexer with repo service name.
			{From: "", To: title, RelationType: "exposes_api"},
		},
	}, nil
}
