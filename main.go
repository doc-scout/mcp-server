package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"docscout-mcp/scanner"
	"docscout-mcp/tools"

	"github.com/google/go-github/v60/github"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/oauth2"
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

	// Try Go duration format first (e.g. "10s", "5m", "1h", "1h30m").
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}

	// Fallback: plain integer → minutes.
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return time.Duration(n) * time.Minute
	}

	log.Printf("Invalid SCAN_INTERVAL '%s', using default %s", raw, defaultScanInterval)
	return defaultScanInterval
}

func main() {
	// --- Configuration from environment variables ---
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		log.Fatal("GITHUB_ORG environment variable is required")
	}

	scanInterval := parseScanInterval(os.Getenv("SCAN_INTERVAL"))

	// --- GitHub client with PAT authentication ---
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(httpClient)

	// --- Scanner ---
	sc := scanner.New(ghClient, org, scanInterval)
	sc.Start(ctx)

	// --- MCP Server ---
	mcpServer := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(true),
	)

	// Register tools.
	tools.Register(mcpServer, sc)

	// --- Start Stdio transport ---
	stdio := server.NewStdioServer(mcpServer)

	log.Printf("%s v%s starting (org=%s, scan_interval=%s)\n", serverName, serverVersion, org, scanInterval)
	log.Println("Listening on stdio...")

	if err := stdio.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
