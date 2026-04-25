// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/doc-scout/mcp-server/memory"
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

	db, err := memory.OpenDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open db: %v\n", err)
		os.Exit(1)
	}

	svc := memory.NewMemoryService(db)
	out, err := GenerateReport(svc, ReportConfig{
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
