// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"fmt"
	"strings"
)

// ParsedGoMod holds data extracted from a go.mod file.
type ParsedGoMod struct {
	// ModulePath is the full module path declared in go.mod (e.g. "github.com/myorg/my-service").
	ModulePath string
	// EntityName is the last path segment of ModulePath, used as the graph node name (e.g. "my-service").
	EntityName string
	// GoVersion is the minimum Go toolchain version declared (e.g. "1.22").
	GoVersion string
	// DirectDeps are the full module paths of direct (non-indirect) dependencies.
	DirectDeps []string
}

// ParseGoMod parses raw go.mod bytes and returns the extracted dependency metadata.
// Only direct (non-indirect) requires are included in DirectDeps.
func ParseGoMod(data []byte) (ParsedGoMod, error) {
	var result ParsedGoMod
	inRequireBlock := false

	for rawLine := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(rawLine)

		switch {
		case strings.HasPrefix(line, "module "):
			result.ModulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			result.EntityName = moduleEntityName(result.ModulePath)

		case strings.HasPrefix(line, "go ") && !inRequireBlock:
			result.GoVersion = strings.TrimSpace(strings.TrimPrefix(line, "go "))

		case line == "require (":
			inRequireBlock = true

		case inRequireBlock && line == ")":
			inRequireBlock = false

		case inRequireBlock:
			if dep := parseDep(line); dep != "" {
				result.DirectDeps = append(result.DirectDeps, dep)
			}

		case strings.HasPrefix(line, "require "):
			// Single-line require outside a block.
			if dep := parseDep(strings.TrimPrefix(line, "require ")); dep != "" {
				result.DirectDeps = append(result.DirectDeps, dep)
			}
		}
	}

	if result.ModulePath == "" {
		return ParsedGoMod{}, fmt.Errorf("go.mod: missing module declaration")
	}
	return result, nil
}

// parseDep returns the module path from a require line, or "" if the line is
// empty, a comment, or an indirect dependency.
func parseDep(line string) string {
	if line == "" || strings.HasPrefix(line, "//") {
		return ""
	}
	if strings.Contains(line, "// indirect") {
		return ""
	}
	parts := strings.Fields(line)
	if len(parts) < 1 {
		return ""
	}
	return parts[0]
}

// moduleEntityName returns the last path segment of a Go module path,
// which serves as the human-readable entity name in the knowledge graph.
// e.g. "github.com/myorg/my-service" → "my-service"
func moduleEntityName(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	return parts[len(parts)-1]
}

// goModParser implements FileParser for go.mod files.
type goModParser struct{}

func (*goModParser) FileType() string    { return "gomod" }
func (*goModParser) Filenames() []string { return []string{"go.mod"} }
func (p *goModParser) Parse(data []byte) (ParsedFile, error) {
	parsed, err := ParseGoMod(data)
	if err != nil {
		return ParsedFile{}, err
	}

	obs := []string{
		"go_module:" + parsed.ModulePath,
	}
	if parsed.GoVersion != "" {
		obs = append(obs, "go_version:"+parsed.GoVersion)
	}

	rels := make([]ParsedRelation, 0, len(parsed.DirectDeps))
	for _, dep := range parsed.DirectDeps {
		rels = append(rels, ParsedRelation{
			From:         parsed.EntityName,
			To:           moduleEntityName(dep),
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

// GoModParser returns the FileParser for go.mod files.
func GoModParser() FileParser { return &goModParser{} }
