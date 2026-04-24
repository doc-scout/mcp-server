// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// serviceEnvPatterns matches env variable names that suggest a service dependency.
var serviceEnvPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(.+)_SERVICE_HOST$`),
	regexp.MustCompile(`^(.+)_SERVICE_URL$`),
	regexp.MustCompile(`^(.+)_API_URL$`),
	regexp.MustCompile(`^(.+)_BASE_URL$`),
}

// k8sDeployment is the minimal shape we need from a K8s Deployment manifest.
type k8sDeployment struct {
	Kind string `yaml:"kind"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Env []struct {
						Name  string `yaml:"name"`
						Value string `yaml:"value"`
					} `yaml:"env"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// k8sServiceParser implements FileParser for Kubernetes Deployment manifests.
type k8sServiceParser struct{}

// K8sServiceParser returns the FileParser for K8s-classified YAML files (k8s type).
func K8sServiceParser() FileParser { return &k8sServiceParser{} }

func (*k8sServiceParser) FileType() string { return "k8s" }

// Filenames returns path-suffix sentinels. K8s files are discovered by the infra
// scanner and classified as "k8s" by classifyFile; these sentinels ensure the
// registry lookup finds this parser for files of that type.
func (*k8sServiceParser) Filenames() []string { return []string{"/k8s/", "/kubernetes/"} }

func (p *k8sServiceParser) Parse(data []byte) (ParsedFile, error) {
	var doc k8sDeployment
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Non-parseable YAML is silently skipped.
		return ParsedFile{}, nil
	}
	if doc.Kind != "Deployment" {
		return ParsedFile{}, nil
	}

	seen := make(map[string]bool)
	var rels []ParsedRelation

	for _, container := range doc.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if !matchesServicePattern(env.Name) {
				continue
			}
			target := extractHostname(env.Value)
			if target == "" || seen[target] {
				continue
			}
			seen[target] = true
			rels = append(rels, ParsedRelation{
				From:         "", // indexer fills with repo service name
				To:           target,
				RelationType: "calls_service",
				Confidence:   "inferred",
			})
		}
	}

	if len(rels) == 0 {
		return ParsedFile{}, nil
	}

	return ParsedFile{
		Observations: []string{"_integration_source:k8s-env"},
		Relations:    rels,
	}, nil
}

// matchesServicePattern returns true if the env var name suggests a service dependency.
func matchesServicePattern(envName string) bool {
	for _, pat := range serviceEnvPatterns {
		if pat.MatchString(envName) {
			return true
		}
	}
	return false
}

// extractHostname extracts a normalized service/hostname from an env var value.
// Handles plain hostnames ("payment-service"), URLs ("http://fraud-service:8080"),
// and returns "" for empty or placeholder values.
func extractHostname(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "${") {
		return ""
	}
	// Strip URL scheme: "http://fraud-service:8080" → "fraud-service:8080"
	if idx := strings.Index(value, "://"); idx >= 0 {
		value = value[idx+3:]
	}
	// Strip path: "fraud-service:8080/path" → "fraud-service:8080"
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}
	// Strip port: "fraud-service:8080" → "fraud-service"
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		value = value[:idx]
	}
	return strings.ToLower(strings.TrimSpace(value))
}
