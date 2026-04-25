// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
	corecontent "github.com/doc-scout/mcp-server/internal/core/content"
)

// IngestURLArgs are the inputs for the ingest_url tool.

type IngestURLArgs struct {
	URL string `json:"url"                   jsonschema:"required,The full URL to fetch and ingest (must start with http:// or https://)."`

	EntityName string `json:"entity_name,omitempty" jsonschema:"Optional name for the graph entity. Defaults to the page title if omitted."`

	EntityType string `json:"entity_type,omitempty" jsonschema:"Entity type to assign (e.g. 'api', 'service', 'doc'). Defaults to 'doc'."`
}

// IngestURLResult is the output of the ingest_url tool.

type IngestURLResult struct {
	EntityName string `json:"entity_name"`

	URL string `json:"url"`

	ObservationCount int `json:"observation_count"`

	Cached bool `json:"cached"`

	Observations []string `json:"observations"`
}

// --- per-domain rate limiter ---

// ingestDomainLimiter tracks last-call time per domain to enforce 5 req/s max.

type ingestDomainLimiter struct {
	mu sync.Mutex

	last map[string]time.Time
}

var domainLimiter = &ingestDomainLimiter{last: make(map[string]time.Time)}

// wait blocks until at least 200ms have passed since the last call to this domain (≤5 req/s).

func (l *ingestDomainLimiter) wait(domain string) {

	const minGap = 200 * time.Millisecond

	l.mu.Lock()

	last, ok := l.last[domain]

	now := time.Now()

	if ok {

		elapsed := now.Sub(last)

		if elapsed < minGap {

			l.mu.Unlock()

			time.Sleep(minGap - elapsed)

			l.mu.Lock()

		}

	}

	l.last[domain] = time.Now()

	l.mu.Unlock()

}

// --- allowlist cache (parsed from ALLOWED_INGEST_DOMAINS once per process) ---

var (
	allowlistOnce sync.Once

	allowlistDomains []string

	allowlistEnabled bool
)

func getAllowlist() ([]string, bool) {

	allowlistOnce.Do(func() {

		raw := strings.TrimSpace(os.Getenv("ALLOWED_INGEST_DOMAINS"))

		if raw == "" {

			allowlistEnabled = false

			return

		}

		allowlistEnabled = true

		for _, d := range strings.Split(raw, ",") {

			d = strings.TrimSpace(d)

			if d != "" {

				allowlistDomains = append(allowlistDomains, strings.ToLower(d))

			}

		}

	})

	return allowlistDomains, allowlistEnabled

}

// --- HTML extraction helpers ---

var (
	reTitleTag = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)

	reHeadingTag = regexp.MustCompile(`(?i)<(h[1-3])[^>]*>(.*?)</h[1-3]>`)

	reMetaDesc = regexp.MustCompile(`(?i)<meta[^>]+name=["']description["'][^>]+content=["']([^"']+)["']`)

	reMetaDescRev = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']description["']`)

	reHTMLTag = regexp.MustCompile(`<[^>]+>`)

	reWhitespace = regexp.MustCompile(`\s+`)

	reEntityName = regexp.MustCompile(`[^a-zA-Z0-9._\-]+`)
)

// stripHTML removes all HTML tags from s.

func stripHTML(s string) string {

	return reHTMLTag.ReplaceAllString(s, " ")

}

// cleanText collapses whitespace and trims.

func cleanText(s string) string {

	return strings.TrimSpace(reWhitespace.ReplaceAllString(stripHTML(s), " "))

}

// extractTitle returns the content of the <title> tag, or "" if not found.

func extractTitle(body string) string {

	if m := reTitleTag.FindStringSubmatch(body); len(m) > 1 {

		return cleanText(m[1])

	}

	return ""

}

// extractHeadings returns observations of the form "heading:<text>" for h1–h3 tags.

func extractHeadings(body string) []string {

	matches := reHeadingTag.FindAllStringSubmatch(body, 50) // cap at 50 headings

	var out []string

	for _, m := range matches {

		if len(m) > 2 {

			text := cleanText(m[2])

			if text != "" {

				out = append(out, "heading:"+text)

			}

		}

	}

	return out

}

// extractMetaDescription returns an observation "description:<content>" or "".

func extractMetaDescription(body string) string {

	if m := reMetaDesc.FindStringSubmatch(body); len(m) > 1 {

		text := strings.TrimSpace(m[1])

		if text != "" {

			return "description:" + text

		}

	}

	// Try reversed attribute order.

	if m := reMetaDescRev.FindStringSubmatch(body); len(m) > 1 {

		text := strings.TrimSpace(m[1])

		if text != "" {

			return "description:" + text

		}

	}

	return ""

}

// estimateWordCount counts whitespace-separated tokens in the visible text.

func estimateWordCount(body string) int {

	visible := stripHTML(body)

	count := 0

	inWord := false

	for _, r := range visible {

		if unicode.IsSpace(r) {

			inWord = false

		} else {

			if !inWord {

				count++

				inWord = true

			}

		}

	}

	return count

}

// sanitizeEntityName turns an arbitrary string into a valid entity name

// matching [a-zA-Z0-9._-]{1,253}.

func sanitizeEntityName(s string) string {

	s = strings.TrimSpace(s)

	if s == "" {

		return ""

	}

	// Replace forbidden chars with "-".

	s = reEntityName.ReplaceAllString(s, "-")

	// Trim leading/trailing dashes.

	s = strings.Trim(s, "-")

	if len(s) > 253 {

		s = s[:253]

	}

	return s

}

// ingestURLHandler returns the MCP handler for the ingest_url tool.

func ingestURLHandler(graph GraphStore, cache corecontent.ContentRepository) func(ctx context.Context, req *mcp.CallToolRequest, args IngestURLArgs) (*mcp.CallToolResult, IngestURLResult, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args IngestURLArgs) (*mcp.CallToolResult, IngestURLResult, error) {

		// 1. Validate URL scheme.

		rawURL := strings.TrimSpace(args.URL)

		if rawURL == "" {

			return nil, IngestURLResult{}, fmt.Errorf("url is required")

		}

		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {

			return nil, IngestURLResult{}, fmt.Errorf("url must start with http:// or https://; got: %s", rawURL)

		}

		parsed, err := url.Parse(rawURL)

		if err != nil || parsed.Host == "" {

			return nil, IngestURLResult{}, fmt.Errorf("invalid url: %s", rawURL)

		}

		domain := strings.ToLower(parsed.Hostname())

		// 2. Check allowlist.

		allowDomains, allowEnabled := getAllowlist()

		if allowEnabled {

			allowed := false

			for _, d := range allowDomains {

				if domain == d || strings.HasSuffix(domain, "."+d) {

					allowed = true

					break

				}

			}

			if !allowed {

				return nil, IngestURLResult{}, fmt.Errorf("domain %q is not in the ALLOWED_INGEST_DOMAINS allowlist", domain)

			}

		}

		// 3. Rate-limit per domain.

		domainLimiter.wait(domain)

		// 4. Fetch with 15s timeout.

		httpClient := &http.Client{Timeout: 15 * time.Second}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)

		if err != nil {

			return nil, IngestURLResult{}, fmt.Errorf("failed to create request: %w", err)

		}

		httpReq.Header.Set("User-Agent", "docscout-mcp/1.0")

		resp, err := httpClient.Do(httpReq)

		if err != nil {

			return nil, IngestURLResult{}, fmt.Errorf("failed to fetch url %s: %w", rawURL, err)

		}

		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {

			return nil, IngestURLResult{}, fmt.Errorf("url %s returned HTTP %d", rawURL, resp.StatusCode)

		}

		// Read up to 1 MB of body to avoid excessive memory use.

		const maxBodyBytes = 1 << 20 // 1 MB

		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))

		if err != nil {

			return nil, IngestURLResult{}, fmt.Errorf("failed to read response body: %w", err)

		}

		body := string(bodyBytes)

		// 5. Extract metadata.

		title := extractTitle(body)

		headings := extractHeadings(body)

		metaDesc := extractMetaDescription(body)

		wordCount := estimateWordCount(body)

		// Build observations.

		var obs []string

		obs = append(obs, fmt.Sprintf("_ingest_url:%s", rawURL))

		if title != "" {

			obs = append(obs, "title:"+title)

		}

		if metaDesc != "" {

			obs = append(obs, metaDesc)

		}

		obs = append(obs, headings...)

		obs = append(obs, fmt.Sprintf("word_count:%d", wordCount))

		// 6. Resolve entity name.

		entityName := strings.TrimSpace(args.EntityName)

		if entityName == "" {

			entityName = sanitizeEntityName(title)

		} else {

			entityName = sanitizeEntityName(entityName)

		}

		if entityName == "" {

			// Fallback: derive from URL path.

			entityName = sanitizeEntityName(domain + "-" + strings.Trim(parsed.Path, "/"))

		}

		if entityName == "" {

			entityName = sanitizeEntityName(domain)

		}

		entityType := strings.TrimSpace(args.EntityType)

		if entityType == "" {

			entityType = "doc"

		}

		// Sanitize observations through the guard layer.

		validObs, _ := sanitizeObservations(entityName, obs)

		// 7. Create or update entity in graph.

		entities, err := graph.CreateEntities([]coregraph.Entity{

			{

				Name: entityName,

				EntityType: entityType,

				Observations: validObs,
			},
		})

		if err != nil {

			return nil, IngestURLResult{}, fmt.Errorf("failed to create entity %q: %w", entityName, err)

		}

		// If entity already existed (empty returned slice), add observations instead.

		if len(entities) == 0 {

			_, err = graph.AddObservations([]coregraph.Observation{

				{EntityName: entityName, Contents: validObs},
			})

			if err != nil {

				slog.Warn("[ingest_url] failed to add observations to existing entity", "entity", entityName, "error", err)

			}

		}

		// 8. Store in content cache if available.

		cached := false

		if cache != nil {

			if upsertErr := cache.Upsert(rawURL, rawURL, "", body, "html"); upsertErr != nil {

				slog.Warn("[ingest_url] failed to cache content", "url", rawURL, "error", upsertErr)

			} else {

				cached = true

			}

		}

		return nil, IngestURLResult{

			EntityName: entityName,

			URL: rawURL,

			ObservationCount: len(validObs),

			Cached: cached,

			Observations: validObs,
		}, nil

	}

}


