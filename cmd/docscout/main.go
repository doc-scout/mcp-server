// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

// Command docscout starts the DocScout MCP server.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	benchmarkcmd "github.com/doc-scout/mcp-server/benchmark/cmd"
	"github.com/doc-scout/mcp-server/internal/app"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--benchmark" {
		os.Exit(benchmarkcmd.Run(os.Args[2:]))
	}

	// Log to stderr to prevent MCP stdio corruption.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := app.LoadConfig()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	components, err := app.Wire(ctx, cfg)
	if err != nil {
		slog.Error("Startup error", "error", err)
		os.Exit(1)
	}

	if err := app.Run(ctx, components); err != nil {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}
}
