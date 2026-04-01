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
