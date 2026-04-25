// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// ParsedPom holds data extracted from a Maven pom.xml file.

type ParsedPom struct {

	// GroupID is the Maven groupId (e.g. "com.myorg").

	GroupID string

	// ArtifactID is the Maven artifactId, used as the graph entity name (e.g. "my-service").

	ArtifactID string

	// EntityName is the artifactId, used as the knowledge graph node name.

	EntityName string

	// Version is the declared artifact version (e.g. "1.0.0").

	Version string

	// DirectDeps are the artifactIds of runtime/compile-scope dependencies.

	// Test-scoped and provided-scoped dependencies are excluded.

	DirectDeps []string
}

// pomDependency mirrors a <dependency> block inside <dependencies>.

type pomDependency struct {
	GroupID string `xml:"groupId"`

	ArtifactID string `xml:"artifactId"`

	Scope string `xml:"scope"`
}

// rawPom is the minimal XML structure we decode from pom.xml.

type rawPom struct {
	XMLName xml.Name `xml:"project"`

	GroupID string `xml:"groupId"`

	ArtifactID string `xml:"artifactId"`

	Version string `xml:"version"`

	Dependencies []pomDependency `xml:"dependencies>dependency"`
}

// ParsePom parses raw pom.xml bytes and returns the extracted Maven metadata.

// Dependencies with scope "test" or "provided" are excluded from DirectDeps,

// mirroring the convention of other parsers that skip non-runtime dependencies.

func ParsePom(data []byte) (ParsedPom, error) {

	var raw rawPom

	if err := xml.Unmarshal(data, &raw); err != nil {

		return ParsedPom{}, fmt.Errorf("pom.xml parse error: %w", err)

	}

	if raw.ArtifactID == "" {

		return ParsedPom{}, fmt.Errorf("pom.xml: missing artifactId")

	}

	deps := make([]string, 0, len(raw.Dependencies))

	for _, d := range raw.Dependencies {

		if d.ArtifactID == "" {

			continue

		}

		scope := strings.ToLower(strings.TrimSpace(d.Scope))

		if scope == "test" || scope == "provided" {

			continue

		}

		deps = append(deps, d.ArtifactID)

	}

	return ParsedPom{

		GroupID: raw.GroupID,

		ArtifactID: raw.ArtifactID,

		EntityName: raw.ArtifactID,

		Version: raw.Version,

		DirectDeps: deps,
	}, nil

}

// pomParser implements FileParser for pom.xml files.

type pomParser struct{}

func (*pomParser) FileType() string { return "pomxml" }

func (*pomParser) Filenames() []string { return []string{"pom.xml"} }

func (p *pomParser) Parse(data []byte) (ParsedFile, error) {

	parsed, err := ParsePom(data)

	if err != nil {

		return ParsedFile{}, err

	}

	obs := []string{

		"maven_artifact:" + parsed.GroupID + ":" + parsed.ArtifactID,
	}

	if parsed.GroupID != "" {

		obs = append(obs, "java_group:"+parsed.GroupID)

	}

	if parsed.Version != "" {

		obs = append(obs, "version:"+parsed.Version)

	}

	rels := make([]ParsedRelation, 0, len(parsed.DirectDeps))

	for _, dep := range parsed.DirectDeps {

		rels = append(rels, ParsedRelation{

			From: parsed.EntityName,

			To: dep,

			RelationType: "depends_on",
		})

	}

	return ParsedFile{

		EntityName: parsed.EntityName,

		EntityType: "service",

		Observations: obs,

		Relations: rels,
	}, nil

}

// PomParser returns the FileParser for pom.xml files.

func PomParser() FileParser { return &pomParser{} }
