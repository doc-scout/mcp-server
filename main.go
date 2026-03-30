// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leonancarvalho/docscout-mcp/indexer"
	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/leonancarvalho/docscout-mcp/scanner"
	"github.com/leonancarvalho/docscout-mcp/tools"
)

const (
	serverName          = "DocScout-MCP"
	serverVersion       = "1.0.0"
	defaultScanInterval = 30 * time.Minute
	defaultMaxContent   = 200 * 1024 // 200 KB
)

func parseScanInterval(raw string) time.Duration {
	if raw == "" {
		return defaultScanInterval
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return time.Duration(n) * time.Minute
	}
	log.Printf("Invalid SCAN_INTERVAL '%s', using default %s", raw, defaultScanInterval)
	return defaultScanInterval
}

func parseCSVEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// isInMemoryDB returns true when the DB URL refers to an in-memory SQLite instance.
func isInMemoryDB(dbURL string) bool {
	return dbURL == "" || strings.Contains(dbURL, ":memory:")
}

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		log.Fatal("GITHUB_ORG environment variable is required")
	}

	scanInterval := parseScanInterval(os.Getenv("SCAN_INTERVAL"))
	targetFiles := parseCSVEnv(os.Getenv("SCAN_FILES"))
	scanDirs := parseCSVEnv(os.Getenv("SCAN_DIRS"))
	extraRepos := parseCSVEnv(os.Getenv("EXTRA_REPOS"))
	repoTopics := parseCSVEnv(os.Getenv("REPO_TOPICS"))

	var repoRegex *regexp.Regexp
	if rx := os.Getenv("REPO_REGEX"); rx != "" {
		compiled, err := regexp.Compile(rx)
		if err != nil {
			log.Fatalf("Invalid REPO_REGEX '%s': %v", rx, err)
		}
		repoRegex = compiled
	}

	httpAddr := os.Getenv("HTTP_ADDR")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("MEMORY_FILE_PATH") // backward compatibility
	}

	scanContent := strings.EqualFold(os.Getenv("SCAN_CONTENT"), "true")
	maxContentSize := defaultMaxContent
	if raw := os.Getenv("MAX_CONTENT_SIZE"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxContentSize = n
		} else {
			log.Printf("Invalid MAX_CONTENT_SIZE '%s', using default %d", raw, defaultMaxContent)
		}
	}

	// Disable content caching silently when using in-memory SQLite.
	if scanContent && isInMemoryDB(dbURL) {
		log.Println("[main] SCAN_CONTENT=true requires a persistent DATABASE_URL; content caching disabled.")
		scanContent = false
	}

	// --- GitHub client ---
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(httpClient)

	// --- Scanner ---
	sc := scanner.New(ghClient, org, scanInterval, targetFiles, scanDirs, extraRepos, repoTopics, repoRegex)

	// --- Database ---
	db, err := memory.OpenDB(dbURL)
	if err != nil {
		log.Fatalf("[main] Failed to open database: %v", err)
	}

	// --- MCP Server ---
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)

	// --- Memory / Knowledge Graph ---
	memory.Register(mcpServer, db)
	autoWriter := memory.NewAutoWriter(db)

	// --- Content Cache ---
	var contentCache *memory.ContentCache
	if scanContent {
		contentCache = memory.NewContentCache(db, true, maxContentSize)
		log.Printf("[main] Content caching enabled (max file size: %d bytes)", maxContentSize)
	}

	// --- Auto-Indexer ---
	ai := indexer.New(sc, autoWriter, contentCache)
	sc.SetOnScanComplete(func(repos []scanner.RepoInfo) {
		ai.Run(context.Background(), repos)
	})

	// --- Register MCP Tools ---
	tools.Register(mcpServer, sc, autoWriter, contentCache)

	// --- Start scanner (initial + periodic) ---
	sc.Start(ctx)

	log.Printf("%s v%s starting (org=%s, scan_interval=%s)\n", serverName, serverVersion, org, scanInterval)

	// --- Transport ---
	if httpAddr != "" {
		log.Printf("Listening on Streamable HTTP at %s...", httpAddr)
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return mcpServer
		}, nil)
		if err := http.ListenAndServe(httpAddr, handler); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	} else {
		log.Println("Listening on stdio...")
		if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}
}
