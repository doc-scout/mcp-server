// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"bufio"
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// springKafkaParser implements FileParser for Spring Boot Kafka configuration files.

type springKafkaParser struct{}

// SpringKafkaParser returns the FileParser for application.yml/yaml/properties files.

func SpringKafkaParser() FileParser { return &springKafkaParser{} }

func (*springKafkaParser) FileType() string { return "spring-kafka" }

func (*springKafkaParser) Filenames() []string {

	return []string{"application.yml", "application.yaml", "application.properties"}

}

func (p *springKafkaParser) Parse(data []byte) (ParsedFile, error) {

	// Try YAML first; fall back to properties format.

	var rels []ParsedRelation

	if looksLikeYAML(data) {

		rels = parseSpringKafkaYAML(data)

	} else {

		rels = parseSpringKafkaProperties(data)

	}

	if len(rels) == 0 {

		return ParsedFile{}, nil

	}

	return ParsedFile{

		// EntityName is empty: the indexer fills Relations[i].From with the repo

		// service name (via the empty-From sentinel convention).

		Observations: []string{"_integration_source:spring-kafka"},

		Relations: rels,
	}, nil

}

func looksLikeYAML(data []byte) bool {

	for _, line := range strings.Split(string(data), "\n") {

		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {

			continue

		}

		// If the first meaningful line contains ": " or ends with ":", it's YAML.

		return strings.Contains(line, ": ") || strings.HasSuffix(line, ":")

	}

	return false

}

// kafkaYAML is the minimal nested structure we extract from application.yml.

type kafkaYAML struct {
	Spring struct {
		Kafka struct {
			Producer struct {
				Topic string `yaml:"topic"`
			} `yaml:"producer"`

			Consumer struct {
				Topics string `yaml:"topics"`
			} `yaml:"consumer"`
		} `yaml:"kafka"`
	} `yaml:"spring"`
}

func parseSpringKafkaYAML(data []byte) []ParsedRelation {

	var cfg kafkaYAML

	if err := yaml.Unmarshal(data, &cfg); err != nil {

		return nil

	}

	var rels []ParsedRelation

	if t := strings.TrimSpace(cfg.Spring.Kafka.Producer.Topic); t != "" && !isPlaceholder(t) {

		rels = append(rels, ParsedRelation{From: "", To: t, RelationType: "publishes_event", Confidence: "inferred"})

	}

	for _, topic := range splitTopics(cfg.Spring.Kafka.Consumer.Topics) {

		rels = append(rels, ParsedRelation{From: "", To: topic, RelationType: "subscribes_event", Confidence: "inferred"})

	}

	return rels

}

func parseSpringKafkaProperties(data []byte) []ParsedRelation {

	var rels []ParsedRelation

	sc := bufio.NewScanner(bytes.NewReader(data))

	for sc.Scan() {

		line := strings.TrimSpace(sc.Text())

		if line == "" || strings.HasPrefix(line, "#") {

			continue

		}

		key, value, ok := strings.Cut(line, "=")

		if !ok {

			continue

		}

		key = strings.TrimSpace(key)

		value = strings.TrimSpace(value)

		switch key {

		case "spring.kafka.producer.topic":

			if !isPlaceholder(value) {

				rels = append(rels, ParsedRelation{From: "", To: value, RelationType: "publishes_event", Confidence: "inferred"})

			}

		case "spring.kafka.consumer.topics":

			for _, topic := range splitTopics(value) {

				rels = append(rels, ParsedRelation{From: "", To: topic, RelationType: "subscribes_event", Confidence: "inferred"})

			}

		}

	}

	return rels

}

// splitTopics splits a comma-separated topic list, trimming whitespace and skipping placeholders.

func splitTopics(raw string) []string {

	if raw == "" {

		return nil

	}

	var topics []string

	for _, t := range strings.Split(raw, ",") {

		t = strings.TrimSpace(t)

		if t != "" && !isPlaceholder(t) {

			topics = append(topics, t)

		}

	}

	return topics

}

// isPlaceholder returns true for Spring ${ENV_VAR} expressions that cannot be resolved.

func isPlaceholder(s string) bool {

	return strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}")

}
