// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParsedPackageJSON holds data extracted from a package.json file.
type ParsedPackageJSON struct {
	// Name is the package name declared in package.json (e.g. "my-service").
	Name string
	// EntityName is the name sanitized for use as a knowledge graph node.
	// Scoped packages like "@myorg/my-service" are normalized to "my-service".
	EntityName string
	// Version is the declared package version (e.g. "1.2.3").
	Version string
	// DirectDeps are the package names listed under "dependencies" (not devDependencies).
	DirectDeps []string
}

// rawPackageJSON is the minimal shape we care about from package.json.
type rawPackageJSON struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

// ParsePackageJSON parses raw package.json bytes and returns the extracted metadata.
// Only "dependencies" entries are included — "devDependencies" are excluded.
func ParsePackageJSON(data []byte) (ParsedPackageJSON, error) {
	var raw rawPackageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return ParsedPackageJSON{}, fmt.Errorf("package.json parse error: %w", err)
	}
	if raw.Name == "" {
		return ParsedPackageJSON{}, fmt.Errorf("package.json: missing name field")
	}

	entityName := PackageEntityName(raw.Name)

	deps := make([]string, 0, len(raw.Dependencies))
	for dep := range raw.Dependencies {
		deps = append(deps, dep)
	}

	return ParsedPackageJSON{
		Name:       raw.Name,
		EntityName: entityName,
		Version:    raw.Version,
		DirectDeps: deps,
	}, nil
}

// PackageEntityName normalizes an npm package name to a safe graph entity name.
// Scoped names like "@myorg/my-service" become "my-service".
// Names with remaining slashes take the last segment.
func PackageEntityName(name string) string {
	// Strip npm scope prefix: "@myorg/my-service" → "my-service"
	if strings.HasPrefix(name, "@") {
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
	}
	// Take last path segment for any remaining slashes.
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

// packageJSONParser implements FileParser for package.json files.
type packageJSONParser struct{}

func (*packageJSONParser) FileType() string    { return "packagejson" }
func (*packageJSONParser) Filenames() []string { return []string{"package.json"} }
func (p *packageJSONParser) Parse(data []byte) (ParsedFile, error) {
	parsed, err := ParsePackageJSON(data)
	if err != nil {
		return ParsedFile{}, err
	}

	obs := []string{"npm_package:" + parsed.Name}
	if parsed.Version != "" {
		obs = append(obs, "version:"+parsed.Version)
	}

	rels := make([]ParsedRelation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		rels = append(rels, ParsedRelation{
			From:         parsed.EntityName,
			To:           PackageEntityName(dep),
			RelationType: "depends_on",
		})
	}

	return ParsedFile{
		EntityName:   parsed.EntityName,
		EntityType:   "service",
		Observations: obs,
		Relations:    rels,
	}, nil
}

// PackageJSONParser returns the FileParser for package.json files.
func PackageJSONParser() FileParser { return &packageJSONParser{} }
