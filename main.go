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
	serverName    = "DocScout-MCP"
	serverVersion = "1.0.0"
)

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

	scanIntervalMinutes := 30 // default
	if v := os.Getenv("SCAN_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			scanIntervalMinutes = n
		} else {
			log.Printf("Invalid SCAN_INTERVAL '%s', using default %d minutes", v, scanIntervalMinutes)
		}
	}

	scanInterval := time.Duration(scanIntervalMinutes) * time.Minute

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

	log.Printf("%s v%s starting (org=%s, scan_interval=%dm)\n", serverName, serverVersion, org, scanIntervalMinutes)
	log.Println("Listening on stdio...")

	if err := stdio.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
