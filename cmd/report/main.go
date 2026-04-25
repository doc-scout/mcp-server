// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"os"

	infradb "github.com/doc-scout/mcp-server/internal/infra/db"
	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
)

func main() {
	dbPath := flag.String("db", "", "path to SQLite database (required)")
	maxNodes := flag.Int("max-nodes", 20, "max nodes in topology flowchart")
	maxEdges := flag.Int("max-edges", 40, "max edges in topology flowchart")
	repo := flag.String("repo", "", "org/repo label")
	elapsed := flag.Int("elapsed", 0, "scan duration in seconds")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required")
		os.Exit(1)
	}

	database, err := infradb.OpenDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open db: %v\n", err)
		os.Exit(1)
	}

	graphRepo := infradb.NewGraphRepo(database)
	svc := coregraph.NewMemoryService(graphRepo)

	out, err := GenerateReport(svc, database, ReportConfig{
		Repo:     *repo,
		Elapsed:  *elapsed,
		MaxNodes: *maxNodes,
		MaxEdges: *maxEdges,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: generate report: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(out)
}
