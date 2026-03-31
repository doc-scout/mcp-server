// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"strings"
)

// CodeOwner represents a single owner entry extracted from a CODEOWNERS file.
type CodeOwner struct {
	// Raw is the original owner token as written in the file
	// (e.g. "@myorg/backend-team", "@alice", "alice@example.com").
	Raw string
	// EntityName is the normalized identifier used as the knowledge-graph node name
	// (e.g. "backend-team", "alice").
	EntityName string
	// EntityType is "team" for @org/team handles, "person" for @username or e-mail owners.
	EntityType string
}

// ParsedCodeowners holds the deduplicated set of owners found in a CODEOWNERS file.
type ParsedCodeowners struct {
	// UniqueOwners contains every distinct owner token found across all rules.
	UniqueOwners []CodeOwner
}

// ParseCodeowners parses raw CODEOWNERS file bytes and returns the deduplicated set
// of owner entries. It supports the three GitHub CODEOWNERS owner formats:
//
//   - @org/team  → EntityType "team",   EntityName = team slug (last path segment)
//   - @username  → EntityType "person", EntityName = username (without @)
//   - user@email → EntityType "person", EntityName = local part before @
//
// Lines starting with '#' and blank lines are ignored. Patterns without owners
// (lone path entries) are silently skipped.
func ParseCodeowners(data []byte) ParsedCodeowners {
	seen := make(map[string]struct{})
	var owners []CodeOwner

	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			// Pattern with no owners declared — nothing to record.
			continue
		}
		// fields[0] is the path glob pattern; fields[1:] are the owner tokens.
		for _, token := range fields[1:] {
			if _, exists := seen[token]; exists {
				continue
			}
			seen[token] = struct{}{}
			owners = append(owners, classifyOwner(token))
		}
	}

	return ParsedCodeowners{UniqueOwners: owners}
}

// classifyOwner normalises a raw CODEOWNERS token into a CodeOwner with a stable
// EntityName and EntityType suitable for the knowledge graph.
func classifyOwner(raw string) CodeOwner {
	lower := strings.ToLower(raw)

	// @org/team handle — use the team slug (last path segment) as entity name.
	if strings.HasPrefix(lower, "@") && strings.Contains(lower, "/") {
		slug := lower[strings.LastIndex(lower, "/")+1:]
		return CodeOwner{Raw: raw, EntityName: slug, EntityType: "team"}
	}

	// @username handle.
	if strings.HasPrefix(lower, "@") {
		return CodeOwner{Raw: raw, EntityName: strings.TrimPrefix(lower, "@"), EntityType: "person"}
	}

	// E-mail address — use the local part before '@'.
	if idx := strings.Index(lower, "@"); idx > 0 {
		return CodeOwner{Raw: raw, EntityName: lower[:idx], EntityType: "person"}
	}

	// Fallback: treat unrecognized tokens as persons using the token itself.
	return CodeOwner{Raw: raw, EntityName: lower, EntityType: "person"}
}
