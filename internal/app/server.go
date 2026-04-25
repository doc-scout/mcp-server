// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	adaptermcp "github.com/doc-scout/mcp-server/internal/adapter/mcp"
	adapterhttp "github.com/doc-scout/mcp-server/internal/adapter/http"
	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
	corescan "github.com/doc-scout/mcp-server/internal/core/scan"
)

// Run wires the scan callback, registers MCP tools, starts the scanner,
// then blocks on either HTTP or stdio transport until ctx is cancelled.
func Run(ctx context.Context, c *Components) error {
	cfg := c.Cfg

	// --- Register MCP Tools (initial) ---
	registerTools(c)

	// --- Wire scan callback ---
	c.Scanner.SetOnScanComplete(func(repos []corescan.RepoInfo) {
		start := time.Now()
		slog.Info("[indexer] Auto-indexing started", "repos", len(repos))
		c.Indexer.Run(ctx, repos)
		slog.Info("[indexer] Auto-indexing complete", "duration", time.Since(start).String())

		// Re-register tools to trigger MCP tools/list_changed notification.
		registerTools(c)
		slog.Info("Triggered tools/list_changed notification")

		if c.SemanticSrv != nil {
			for _, repo := range repos {
				go c.SemanticSrv.IndexDocs(ctx, repo.FullName)
			}
		}
	})

	// --- Start scanner ---
	c.Scanner.Start(ctx)

	slog.Info("Server starting", "name", "DocScout-MCP", "version", serverVersion, "org", cfg.GitHubOrg, "scan_interval", cfg.ScanInterval)

	if cfg.HTTPAddr != "" {
		return runHTTP(ctx, c)
	}
	return runStdio(ctx, c)
}

func registerTools(c *Components) {
	adaptermcp.Register(
		c.MCPServer,
		c.Scanner,
		c.Graph,
		c.ContentCache,
		c.SemanticSrv,
		c.ToolMetrics,
		c.DocMetrics,
		c.ContentCache,
		c.Cfg.GraphReadOnly,
		c.AuditStore,
	)
}

// serverHealthProvider implements adapterhttp.StatusProvider.
type serverHealthProvider struct {
	c         *Components
	startedAt time.Time
}

func (p *serverHealthProvider) HealthStatus() adapterhttp.Status {
	_, _, repoCount := p.c.Scanner.Status()
	status := "starting"
	if repoCount > 0 {
		status = "ok"
	}
	var entities int64
	if p.c.Graph != nil {
		entities, _ = p.c.Graph.EntityCount()
	}
	return adapterhttp.Status{
		Status:    status,
		StartedAt: p.startedAt,
		RepoCount: repoCount,
		Entities:  entities,
	}
}

func runHTTP(ctx context.Context, c *Components) error {
	cfg := c.Cfg

	slog.Info("Listening on Streamable HTTP", "addr", cfg.HTTPAddr)

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return c.MCPServer
	}, nil)

	healthProvider := &serverHealthProvider{c: c, startedAt: time.Now()}

	mux := http.NewServeMux()
	mux.Handle("/", mcpHandler)
	mux.HandleFunc("/healthz", adapterhttp.HealthHandler(healthProvider))

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		snapshot := c.ToolMetrics.Snapshot()
		fmt.Fprintf(w, "# HELP docscout_tool_calls_total Total number of MCP tool calls since server start.\n")
		fmt.Fprintf(w, "# TYPE docscout_tool_calls_total counter\n")
		for tool, count := range snapshot {
			fmt.Fprintf(w, "docscout_tool_calls_total{tool=%q} %d\n", tool, count)
		}
		fmt.Fprintf(w, "# HELP docscout_document_accesses_total Total number of times a specific document was fetched or returned in content search results since server start.\n")
		fmt.Fprintf(w, "# TYPE docscout_document_accesses_total counter\n")
		for _, d := range c.DocMetrics.TopN(0) {
			fmt.Fprintf(w, "docscout_document_accesses_total{repo=%q,path=%q} %d\n", d.Repo, d.Path, d.Count)
		}
	})

	mux.HandleFunc("/audit", func(w http.ResponseWriter, r *http.Request) {
		if c.AuditStore == nil {
			http.Error(w, `{"error":"audit persistence not enabled — set DATABASE_URL to a persistent store"}`, http.StatusServiceUnavailable)
			return
		}
		filter := coreaudit.AuditFilter{
			Agent:     r.URL.Query().Get("agent"),
			Tool:      r.URL.Query().Get("tool"),
			Operation: r.URL.Query().Get("operation"),
			Outcome:   r.URL.Query().Get("outcome"),
		}
		if s := r.URL.Query().Get("since"); s != "" {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				http.Error(w, `{"error":"invalid since timestamp"}`, http.StatusBadRequest)
				return
			}
			filter.Since = t
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				filter.Limit = n
			}
		}
		events, total, err := c.AuditStore.Query(r.Context(), filter)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if events == nil {
			events = []coreaudit.AuditEvent{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"events": events, "total": total})
	})

	mux.HandleFunc("/audit/summary", func(w http.ResponseWriter, r *http.Request) {
		if c.AuditStore == nil {
			http.Error(w, `{"error":"audit persistence not enabled — set DATABASE_URL to a persistent store"}`, http.StatusServiceUnavailable)
			return
		}
		windows := map[string]time.Duration{
			"1h":  time.Hour,
			"24h": 24 * time.Hour,
			"7d":  7 * 24 * time.Hour,
		}
		window := windows[r.URL.Query().Get("window")]
		if window == 0 {
			window = 24 * time.Hour
		}
		summary, err := c.AuditStore.Summary(r.Context(), window)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
	})

	if cfg.WebhookSecret != "" {
		mux.Handle("/webhook", adapterhttp.WebhookHandler(ctx, []byte(cfg.WebhookSecret), c.Scanner))
		slog.Info("GitHub webhook endpoint enabled", "path", "/webhook")
	}

	// Bearer Token Auth Middleware — bypasses /webhook and /healthz.
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/webhook" || r.URL.Path == "/healthz" {
			mux.ServeHTTP(w, r)
			return
		}
		if cfg.BearerToken != "" {
			provided := r.Header.Get("Authorization")
			expected := "Bearer " + cfg.BearerToken
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      authHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
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
	return srv.Shutdown(shutdownCtx)
}

func runStdio(ctx context.Context, c *Components) error {
	slog.Info("Listening on stdio...")
	go func() {
		<-ctx.Done()
		slog.Info("Received shutdown signal, exiting...")
		os.Exit(0)
	}()
	if err := c.MCPServer.Run(ctx, &mcpsdk.StdioTransport{}); err != nil && err != context.Canceled {
		return err
	}
	return nil
}
