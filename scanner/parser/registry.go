// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"fmt"
	"sync"
)

// Default is the global registry. Built-in parsers register here from main.go.

// Custom parsers may register here from their own init() functions.

var Default = NewRegistry()

// Register adds p to the global Default registry.

// Panics on duplicate FileType() or Filenames() overlap — fail-fast at startup.

func Register(p FileParser) { Default.Register(p) }

// RegisterDefaults registers all built-in parsers on reg.
// Callers that need an isolated registry (e.g. the benchmark CLI) use this
// instead of the package-level Register helpers that modify Default.
func RegisterDefaults(reg *ParserRegistry) {
	reg.Register(GoModParser())
	reg.Register(PackageJSONParser())
	reg.Register(PomParser())
	reg.Register(CodeownersParser())
	reg.Register(CatalogParser())
	reg.Register(AsyncAPIParser())
	reg.Register(SpringKafkaParser())
	reg.Register(OpenAPIParser())
	reg.Register(ProtoParser())
	reg.Register(K8sServiceParser())
}

// ParserRegistry is a thread-safe map of FileParser implementations keyed by FileType().

type ParserRegistry struct {
	mu sync.RWMutex

	parsers map[string]FileParser // keyed by FileType()

	filenames map[string]string // filename → FileType(), for duplicate detection

}

// NewRegistry returns an empty ParserRegistry.

func NewRegistry() *ParserRegistry {

	return &ParserRegistry{

		parsers: make(map[string]FileParser),

		filenames: make(map[string]string),
	}

}

// Register adds p to the registry.

// Panics if p.FileType() is already registered.

// Panics if any entry in p.Filenames() is already claimed by another parser.

func (r *ParserRegistry) Register(p FileParser) {

	r.mu.Lock()

	defer r.mu.Unlock()

	ft := p.FileType()

	if _, exists := r.parsers[ft]; exists {

		panic(fmt.Sprintf("parser registry: duplicate FileType %q", ft))

	}

	for _, fn := range p.Filenames() {

		if existing, clash := r.filenames[fn]; clash {

			panic(fmt.Sprintf("parser registry: filename %q already claimed by %q", fn, existing))

		}

	}

	r.parsers[ft] = p

	for _, fn := range p.Filenames() {

		r.filenames[fn] = ft

	}

}

// Get returns the FileParser for the given fileType, or (nil, false) if not registered.

func (r *ParserRegistry) Get(fileType string) (FileParser, bool) {

	r.mu.RLock()

	defer r.mu.RUnlock()

	p, ok := r.parsers[fileType]

	return p, ok

}

// All returns a snapshot of all registered parsers in an unspecified order.

func (r *ParserRegistry) All() []FileParser {

	r.mu.RLock()

	defer r.mu.RUnlock()

	result := make([]FileParser, 0, len(r.parsers))

	for _, p := range r.parsers {

		result = append(result, p)

	}

	return result

}

// TargetFilenames returns the union of all Filenames() across all registered parsers.

func (r *ParserRegistry) TargetFilenames() []string {

	r.mu.RLock()

	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.filenames))

	for fn := range r.filenames {

		result = append(result, fn)

	}

	return result

}
