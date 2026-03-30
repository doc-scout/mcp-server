// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
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
	slog.Warn("Invalid SCAN_INTERVAL, using default", "raw", raw, "default", defaultScanInterval)
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
	// Configure slog to write to stderr to prevent MCP stdio corruption
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		slog.Error("GITHUB_TOKEN environment variable is required")
		os.Exit(1)
	}

	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		slog.Error("GITHUB_ORG environment variable is required")
		os.Exit(1)
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
			slog.Error("Invalid REPO_REGEX", "regex", rx, "error", err)
			os.Exit(1)
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
			slog.Warn("Invalid MAX_CONTENT_SIZE, using default", "raw", raw, "default", defaultMaxContent)
		}
	}

	// Disable content caching silently when using in-memory SQLite.
	if scanContent && isInMemoryDB(dbURL) {
		slog.Warn("SCAN_CONTENT=true requires a persistent DATABASE_URL; content caching disabled.")
		scanContent = false
	}

	// --- Context & Graceful Shutdown ---
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- GitHub client ---
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(httpClient)

	// --- Scanner ---
	sc := scanner.New(ghClient, org, scanInterval, targetFiles, scanDirs, extraRepos, repoTopics, repoRegex)

	// --- Database ---
	db, err := memory.OpenDB(dbURL)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
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
		slog.Info("Content caching enabled", "maxFileSize", maxContentSize)
	}

	// --- Auto-Indexer ---
	ai := indexer.New(sc, autoWriter, contentCache)
	sc.SetOnScanComplete(func(repos []scanner.RepoInfo) {
		ai.Run(context.Background(), repos)
		
		// Map concrete pointers to interface accurately to avoid typed-nils
		var searcher tools.ContentSearcher
		if contentCache != nil {
			searcher = contentCache
		}

		// Re-register tools to implicitly trigger the MCP tools/list_changed notification
		tools.Register(mcpServer, sc, autoWriter, searcher)
		slog.Info("Triggered tools/list_changed notification")
	})

	// --- Register MCP Tools ---
	var searcher tools.ContentSearcher
	if contentCache != nil {
		searcher = contentCache
	}
	tools.Register(mcpServer, sc, autoWriter, searcher)

	// --- Start scanner (initial + periodic) ---
	sc.Start(ctx)

	slog.Info("Server starting", "name", serverName, "version", serverVersion, "org", org, "scan_interval", scanInterval)

	// --- Transport ---
	if httpAddr != "" {
		slog.Info("Listening on Streamable HTTP", "addr", httpAddr)
		mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return mcpServer
		}, nil)

		// Basic Bearer Token Auth Middleware
		authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expectedToken := os.Getenv("MCP_HTTP_BEARER_TOKEN")
			if expectedToken != "" {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "Bearer "+expectedToken {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			mcpHandler.ServeHTTP(w, r)
		})

		srv := &http.Server{
			Addr:    httpAddr,
			Handler: authHandler,
		}

		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("HTTP server error", "error", err)
				os.Exit(1)
			}
		}()

		<-ctx.Done()
		slog.Info("Shutting down HTTP server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	} else {
		slog.Info("Listening on stdio...")
		go func() {
			<-ctx.Done()
			slog.Info("Received shutdown signal, exiting...")
			os.Exit(0)
		}()
		
		if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
			if err != context.Canceled {
				slog.Error("Server error", "error", err)
				os.Exit(1)
			}
		}
	}
}
