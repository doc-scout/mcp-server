// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package parser

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// asyncAPIDoc is the minimal shape we need from an AsyncAPI document.
type asyncAPIDoc struct {
	Info struct {
		Title string `yaml:"title"`
	} `yaml:"info"`
	Channels map[string]asyncAPIChannel `yaml:"channels"`
}

type asyncAPIChannel struct {
	Publish   *asyncAPIOperation `yaml:"publish"`
	Subscribe *asyncAPIOperation `yaml:"subscribe"`
}

type asyncAPIOperation struct {
	Message struct {
		Name string `yaml:"name"`
	} `yaml:"message"`
}

// asyncAPIParser implements FileParser for AsyncAPI documents.
type asyncAPIParser struct{}

// AsyncAPIParser returns the FileParser for asyncapi.yaml and asyncapi.json files.
func AsyncAPIParser() FileParser { return &asyncAPIParser{} }

func (*asyncAPIParser) FileType() string    { return "asyncapi" }
func (*asyncAPIParser) Filenames() []string { return []string{"asyncapi.yaml", "asyncapi.json"} }

func (p *asyncAPIParser) Parse(data []byte) (ParsedFile, error) {
	var doc asyncAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return ParsedFile{}, fmt.Errorf("asyncapi: yaml parse error: %w", err)
	}
	title := strings.TrimSpace(doc.Info.Title)
	if title == "" {
		return ParsedFile{}, fmt.Errorf("asyncapi: missing info.title")
	}

	var aux []AuxEntity
	var rels []ParsedRelation

	for channelName, ch := range doc.Channels {
		var schemaObs []string
		if ch.Publish != nil && ch.Publish.Message.Name != "" {
			schemaObs = append(schemaObs, "schema:"+ch.Publish.Message.Name)
		} else if ch.Subscribe != nil && ch.Subscribe.Message.Name != "" {
			schemaObs = append(schemaObs, "schema:"+ch.Subscribe.Message.Name)
		}
		aux = append(aux, AuxEntity{
			Name:         channelName,
			EntityType:   "event-topic",
			Observations: schemaObs,
		})

		if ch.Publish != nil {
			rels = append(rels, ParsedRelation{
				From:         title,
				To:           channelName,
				RelationType: "publishes_event",
			})
		}
		if ch.Subscribe != nil {
			rels = append(rels, ParsedRelation{
				From:         title,
				To:           channelName,
				RelationType: "subscribes_event",
			})
		}
	}

	return ParsedFile{
		EntityName:   title,
		EntityType:   "service",
		Observations: []string{"_integration_source:asyncapi"},
		AuxEntities:  aux,
		Relations:    rels,
	}, nil
}
