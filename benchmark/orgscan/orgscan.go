// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

// Package orgscan wires up the full scanner+indexer stack against a live GitHub
// org and collects graph statistics in a single pass. Used by the --benchmark
// --org flag to produce the "Org Scan Stats" section of benchmark/RESULTS.md.
package orgscan

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"

	"github.com/doc-scout/mcp-server/internal/app"
	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
	corescan "github.com/doc-scout/mcp-server/internal/core/scan"
	infradb "github.com/doc-scout/mcp-server/internal/infra/db"
	ghinfra "github.com/doc-scout/mcp-server/internal/infra/github"
	"github.com/doc-scout/mcp-server/internal/infra/github/parser"
)

// OrgStats holds the graph statistics collected after a full org scan.
type OrgStats struct {
	Org          string
	ScanDuration time.Duration
	Repos        int
	EntityTotal  int64
	EntityByType []TypeCount
	RelTotal     int64
	RelByConf    []TypeCount
}

// TypeCount is a label+count pair used in breakdown tables.
type TypeCount struct {
	Label string
	Count int64
}

// Run scans org with the given GitHub token, stores the graph in a temp SQLite
// file, collects stats, and cleans up. ctx is forwarded to the scanner; set a
// sensible deadline (e.g. 30 min) on the caller's context.
func Run(ctx context.Context, org, token string) (*OrgStats, error) {
	if org == "" {
		return nil, fmt.Errorf("orgscan: --org is required")
	}
	if token == "" {
		return nil, fmt.Errorf("orgscan: --token or GITHUB_TOKEN is required")
	}

	tmp, err := os.CreateTemp("", "docscout-bench-*.db")
	if err != nil {
		return nil, fmt.Errorf("orgscan: create temp db: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	database, err := infradb.OpenDB(tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("orgscan: open db: %w", err)
	}
	graphRepo := infradb.NewGraphRepo(database)
	svc := coregraph.NewMemoryService(graphRepo)

	reg := parser.NewRegistry()
	parser.RegisterDefaults(reg)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	ghClient := github.NewClient(oauth2.NewClient(ctx, ts))

	const veryLongInterval = 999 * time.Hour
	sc := ghinfra.New(ghClient, org, veryLongInterval, nil, nil, nil, nil, nil, (*regexp.Regexp)(nil), reg)

	ai := app.New(sc, svc, nil, reg)

	done := make(chan int, 1)
	sc.SetOnScanComplete(func(repos []corescan.RepoInfo) {
		ai.Run(ctx, repos)
		select {
		case done <- len(repos):
		default:
		}
	})

	start := time.Now()
	go sc.Start(ctx)

	var repos int
	select {
	case repos = <-done:
	case <-ctx.Done():
		return nil, fmt.Errorf("orgscan: scan timed out: %w", ctx.Err())
	}

	elapsed := time.Since(start)

	entityTotal, err := svc.EntityCount()
	if err != nil {
		return nil, fmt.Errorf("orgscan: entity count: %w", err)
	}
	typeCounts, err := svc.EntityTypeCounts()
	if err != nil {
		return nil, fmt.Errorf("orgscan: entity type counts: %w", err)
	}

	type confRow struct {
		Confidence string
		Count      int64
	}
	var confRows []confRow
	if err := database.Raw("SELECT COALESCE(NULLIF(confidence,''),'authoritative') AS confidence, COUNT(*) AS count FROM db_relations GROUP BY confidence").Scan(&confRows).Error; err != nil {
		return nil, fmt.Errorf("orgscan: relation confidence counts: %w", err)
	}

	var relTotal int64
	relByConf := make([]TypeCount, 0, len(confRows))
	for _, r := range confRows {
		relTotal += r.Count
		relByConf = append(relByConf, TypeCount{Label: r.Confidence, Count: r.Count})
	}
	sort.Slice(relByConf, func(i, j int) bool { return relByConf[i].Label < relByConf[j].Label })

	entityByType := make([]TypeCount, 0, len(typeCounts))
	for k, v := range typeCounts {
		entityByType = append(entityByType, TypeCount{Label: k, Count: v})
	}
	sort.Slice(entityByType, func(i, j int) bool { return entityByType[i].Label < entityByType[j].Label })

	return &OrgStats{
		Org:          org,
		ScanDuration: elapsed,
		Repos:        repos,
		EntityTotal:  entityTotal,
		EntityByType: entityByType,
		RelTotal:     relTotal,
		RelByConf:    relByConf,
	}, nil
}
