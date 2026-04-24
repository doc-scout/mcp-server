// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

// ParsedRelation is a directed edge produced by a parser.
// If To is empty (""), the indexer fills it with the derived repo service name —
// used by CodeownersParser where the target service is context-dependent.
type ParsedRelation struct {
	From         string
	To           string
	RelationType string // e.g. "depends_on", "owns", "provides_api"
	// Confidence is the edge reliability level. Parsers set "authoritative" for explicit
	// contract files (AsyncAPI, OpenAPI, proto, go.mod, pom.xml, catalog, CODEOWNERS, package.json)
	// and "inferred" for heuristic sources (Spring Kafka, K8s env vars).
	// If empty, the indexer defaults to "authoritative".
	Confidence string
}

// AuxEntity is an additional graph entity produced alongside the primary entity.
// Used by parsers that create multiple entities (e.g. CodeownersParser creates one
// team/person entity per owner).
type AuxEntity struct {
	Name         string
	EntityType   string
	Observations []string
}

// MergeMode controls how upsertParsedFile() handles existing graph entities.
type MergeMode int

const (
	// MergeModeUpsert (default) — create entity if absent, add observations if present.
	MergeModeUpsert MergeMode = iota
	// MergeModeCatalog — same as MergeModeUpsert in the current implementation;
	// reserved to allow catalog-specific merge semantics in a future iteration.
	MergeModeCatalog
)

// ParsedFile is the normalized, graph-ready output every FileParser must return.
// EntityName must be non-empty unless AuxEntities is non-empty (codeowners pattern).
// EntityType defaults to "service" if blank.
// Observations and Relations may be nil or empty.
// MergeMode defaults to MergeModeUpsert if zero.
// Relations with From == "" have their From field filled with the derived repo service name.
// Relations with To == "" have their To field filled with the derived repo service name.
type ParsedFile struct {
	EntityName   string
	EntityType   string
	Observations []string
	Relations    []ParsedRelation
	MergeMode    MergeMode
	// AuxEntities are created/updated before Relations. Used when a single file
	// produces multiple graph entities (e.g. CODEOWNERS produces one per owner).
	AuxEntities []AuxEntity
}

// FileParser is the extension point for manifest parsers.
// All methods must be safe for concurrent use (implementations are typically stateless).
type FileParser interface {
	// FileType returns the classifier key used by classifyFile() and the indexer.
	// Must be unique across the registry. Examples: "gomod", "catalog-info".
	FileType() string

	// Filenames returns the exact filenames (or suffix sentinels starting with ".")
	// this parser handles. The scanner looks for these at the repo root.
	// Examples: ["go.mod"], ["catalog-info.yaml"], [".proto"] (suffix match).
	Filenames() []string

	// Parse converts raw file bytes into a normalized graph-ready result.
	Parse(data []byte) (ParsedFile, error)
}
