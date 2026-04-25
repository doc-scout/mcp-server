// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/doc-scout/mcp-server/internal/infra/embeddings"
)

const (
	defaultScanInterval = 30 * time.Minute
	defaultMaxContent   = 200 * 1024 // 200 KB
)

// Config holds all runtime configuration read from environment variables.
type Config struct {
	GitHubToken    string
	GitHubOrg      string
	ScanInterval   time.Duration
	TargetFiles    []string
	ScanDirs       []string
	InfraDirs      []string
	ExtraRepos     []string
	RepoTopics     []string
	RepoRegex      *regexp.Regexp
	HTTPAddr       string
	DatabaseURL    string
	ScanContent    bool
	GraphReadOnly  bool
	MaxContentSize int
	AgentID        string
	WebhookSecret  string
	BearerToken    string
	EmbedConfig    embeddings.Config
}

// LoadConfig reads and validates all environment variables.
func LoadConfig() (Config, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return Config{}, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		return Config{}, fmt.Errorf("GITHUB_ORG environment variable is required")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("MEMORY_FILE_PATH") // backward compatibility
	}

	scanContent := strings.EqualFold(os.Getenv("SCAN_CONTENT"), "true")
	if scanContent && isInMemoryDB(dbURL) {
		slog.Error("SCAN_CONTENT=true requires a persistent DATABASE_URL. Content caching has been disabled.")
		scanContent = false
	}

	maxContentSize := defaultMaxContent
	if raw := os.Getenv("MAX_CONTENT_SIZE"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxContentSize = n
		} else {
			slog.Warn("Invalid MAX_CONTENT_SIZE, using default", "raw", raw)
		}
	}

	var repoRegex *regexp.Regexp
	if rx := os.Getenv("REPO_REGEX"); rx != "" {
		compiled, err := regexp.Compile(rx)
		if err != nil {
			return Config{}, fmt.Errorf("invalid REPO_REGEX %q: %w", rx, err)
		}
		repoRegex = compiled
	}

	graphReadOnly := strings.EqualFold(os.Getenv("GRAPH_READ_ONLY"), "true")
	if graphReadOnly {
		slog.Info("Graph read-only mode enabled")
	}

	return Config{
		GitHubToken:    token,
		GitHubOrg:      org,
		ScanInterval:   parseScanInterval(os.Getenv("SCAN_INTERVAL")),
		TargetFiles:    parseCSVEnv(os.Getenv("SCAN_FILES")),
		ScanDirs:       parseCSVEnv(os.Getenv("SCAN_DIRS")),
		InfraDirs:      parseCSVEnv(os.Getenv("SCAN_INFRA_DIRS")),
		ExtraRepos:     parseCSVEnv(os.Getenv("EXTRA_REPOS")),
		RepoTopics:     parseCSVEnv(os.Getenv("REPO_TOPICS")),
		RepoRegex:      repoRegex,
		HTTPAddr:       os.Getenv("HTTP_ADDR"),
		DatabaseURL:    dbURL,
		ScanContent:    scanContent,
		GraphReadOnly:  graphReadOnly,
		MaxContentSize: maxContentSize,
		AgentID:        os.Getenv("AGENT_ID"),
		WebhookSecret:  os.Getenv("GITHUB_WEBHOOK_SECRET"),
		BearerToken:    os.Getenv("MCP_HTTP_BEARER_TOKEN"),
		EmbedConfig:    embeddings.ConfigFromEnv(),
	}, nil
}

func isInMemoryDB(dbURL string) bool {
	return dbURL == "" || strings.Contains(dbURL, ":memory:")
}

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
	var result []string
	for p := range strings.SplitSeq(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
