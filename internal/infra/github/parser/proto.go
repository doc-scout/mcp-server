// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"strings"
)

// externalProtoPackages are well-known external proto packages whose imports
// should not produce internal depends_on_grpc relations.
var externalProtoPackages = []string{
	"google/",
	"github.com/",
	"grpc/",
	"protoc-gen-",
}

// protoParser implements FileParser for Protocol Buffer files.
type protoParser struct{}

// ProtoParser returns the FileParser for *.proto files (suffix-matched by scanner).
func ProtoParser() FileParser { return &protoParser{} }

func (*protoParser) FileType() string    { return "proto" }
func (*protoParser) Filenames() []string { return []string{".proto"} }

func (p *protoParser) Parse(data []byte) (ParsedFile, error) {
	var aux []AuxEntity
	var rels []ParsedRelation
	var packageName string

	for rawLine := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(rawLine)

		if strings.HasPrefix(line, "package ") {
			packageName = strings.TrimSuffix(strings.TrimPrefix(line, "package "), ";")
			packageName = strings.TrimSpace(packageName)
			continue
		}

		if strings.HasPrefix(line, "service ") {
			// "service Foo {" → extract "Foo"
			rest := strings.TrimPrefix(line, "service ")
			name := strings.Fields(rest)[0]
			name = strings.TrimSuffix(name, "{")
			name = strings.TrimSpace(name)
			if name != "" {
				aux = append(aux, AuxEntity{
					Name:       name,
					EntityType: "grpc-service",
				})
				// From is empty: indexer fills with repo service name.
				rels = append(rels, ParsedRelation{
					From:         "",
					To:           name,
					RelationType: "provides_grpc",
					Confidence:   "authoritative",
				})
			}
			continue
		}

		if strings.HasPrefix(line, `import "`) {
			// `import "path/to/service.proto";` → extract base name without extension.
			importPath := strings.TrimPrefix(line, `import "`)
			importPath = strings.TrimSuffix(importPath, `";`)
			importPath = strings.TrimSpace(importPath)
			if isExternalProtoImport(importPath) {
				continue
			}
			// Extract base name: "fraud/fraud_service.proto" → "fraud_service"
			base := importPath
			if idx := strings.LastIndex(base, "/"); idx >= 0 {
				base = base[idx+1:]
			}
			base = strings.TrimSuffix(base, ".proto")
			if base != "" {
				rels = append(rels, ParsedRelation{
					From:         "",
					To:           base,
					RelationType: "depends_on_grpc",
					Confidence:   "authoritative",
				})
			}
		}
	}

	if len(aux) == 0 && len(rels) == 0 {
		return ParsedFile{}, nil
	}

	obs := []string{"_integration_source:proto"}
	if packageName != "" {
		obs = append(obs, "proto_package:"+packageName)
	}

	return ParsedFile{
		Observations: obs,
		AuxEntities:  aux,
		Relations:    rels,
	}, nil
}

func isExternalProtoImport(path string) bool {
	for _, prefix := range externalProtoPackages {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
