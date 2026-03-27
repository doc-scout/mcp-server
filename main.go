package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"net/http"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docscout-mcp/memory"
	"docscout-mcp/scanner"
	"docscout-mcp/tools"
)

const (
	serverName          = "DocScout-MCP"
	serverVersion       = "1.0.0"
	defaultScanInterval = 30 * time.Minute
)

// parseScanInterval accepts Go duration strings ("10s", "5m", "1h30m") or
// plain integers which are treated as minutes for backward compatibility.
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

// parseCSVEnv splits a comma-separated env var into trimmed, non-empty values.
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

	httpAddr := os.Getenv("HTTP_ADDR") // e.g. ":8080"
	// memoryFile := os.Getenv("MEMORY_FILE_PATH") // Will be implemented in the next step

	// --- GitHub client with PAT authentication ---
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(httpClient)

	// --- Scanner ---
	sc := scanner.New(ghClient, org, scanInterval, targetFiles, scanDirs, extraRepos, repoTopics, repoRegex)
	sc.Start(ctx)

	// --- MCP Server Initialization ---
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)

	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		dbUrl = os.Getenv("MEMORY_FILE_PATH") // backward compatibility fallback
	}

	// Register tools
	tools.Register(mcpServer, sc)
	memory.Register(mcpServer, dbUrl)

	log.Printf("%s v%s starting (org=%s, scan_interval=%s)\n", serverName, serverVersion, org, scanInterval)

	// --- Transport Selection ---
	if httpAddr != "" {
		log.Printf("Listening on HTTP SSE at %s...", httpAddr)
		
		handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server {
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
