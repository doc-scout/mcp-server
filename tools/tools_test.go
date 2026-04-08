// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leonancarvalho/docscout-mcp/memory"
	"github.com/leonancarvalho/docscout-mcp/scanner"
)

// mockScanner implements DocumentScanner for testing
type mockScanner struct {
	repos   []scanner.RepoInfo
	files   []scanner.FileEntry
	content map[string]string // key: repoName/path
}

func (m *mockScanner) ListRepos() []scanner.RepoInfo {
	return m.repos
}

func (m *mockScanner) SearchDocs(query string) []scanner.FileEntry {
	return m.files
}

func (m *mockScanner) GetFileContent(ctx context.Context, repo, path string) (string, error) {
	key := fmt.Sprintf("%s/%s", repo, path)
	if content, ok := m.content[key]; ok {
		return content, nil
	}
	return "", fmt.Errorf("security policy: path '%s' is not indexed as a documentation file", path)
}

func (m *mockScanner) Status() (bool, time.Time, int) {
	return false, time.Time{}, len(m.repos)
}

func TestListReposHandler(t *testing.T) {
	mock := &mockScanner{
		repos: []scanner.RepoInfo{
			{
				Name:        "test-org/test-repo",
				FullName:    "test-org/test-repo",
				Description: "A test repo",
				HTMLURL:     "https://github.com/test-org/test-repo",
				Files: []scanner.FileEntry{
					{RepoName: "test-repo", Path: "README.md", Type: "readme"},
				},
			},
		},
	}

	handler := listReposHandler(mock)
	req := &mcp.CallToolRequest{}

	// The new SDK generic API signature
	res, output, err := handler(t.Context(), req, ListReposArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We expect SDK auto-marshaling, so CallToolResult is nil
	if res != nil && res.IsError {
		t.Fatalf("expected IsError false")
	}

	summaries := output.Repos
	if len(summaries) != 1 {
		t.Fatalf("expected 1 repo summary, got %d", len(summaries))
	}
	if summaries[0].Name != "test-org/test-repo" {
		t.Errorf("expected repo name 'test-org/test-repo', got %s", summaries[0].Name)
	}
	if len(summaries[0].FileTypes) != 1 || summaries[0].FileTypes[0] != "readme" {
		t.Errorf("expected FileTypes ['readme'], got %v", summaries[0].FileTypes)
	}
}

func TestSearchDocsHandler(t *testing.T) {
	mock := &mockScanner{
		files: []scanner.FileEntry{
			{RepoName: "test-repo", Path: "docs/guide.md", Type: "docs"},
		},
	}

	handler := searchDocsHandler(mock)
	req := &mcp.CallToolRequest{}

	res, output, err := handler(t.Context(), req, SearchDocsArgs{Query: "guide"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res != nil && res.IsError {
		t.Fatalf("expected IsError false")
	}

	results := output.Files
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Path != "docs/guide.md" {
		t.Errorf("expected path 'docs/guide.md', got %s", results[0].Path)
	}
}

func TestGetFileContentHandler(t *testing.T) {
	mock := &mockScanner{
		content: map[string]string{
			"test-repo/docs/guide.md": "# Guide\nThis is a guide.",
		},
	}

	handler := getFileContentHandler(mock, NewDocMetrics())

	// Test successful case
	req := &mcp.CallToolRequest{}

	res, output, err := handler(t.Context(), req, GetFileContentArgs{Repo: "test-repo", Path: "docs/guide.md"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res != nil && res.IsError {
		t.Fatalf("expected IsError false")
	}

	if output.Content != "# Guide\nThis is a guide." {
		t.Errorf("unexpected content: %s", output.Content)
	}

	// Test error case (file not indexed)
	req2 := &mcp.CallToolRequest{}

	_, _, err2 := handler(t.Context(), req2, GetFileContentArgs{Repo: "test-repo", Path: "docs/secret.txt"})

	// The new behavior returns a standard Go compilation error handled by the wrapper.
	if err2 == nil {
		t.Errorf("expected error for unindexed file")
	}
}

type mockContentSearcher struct {
	matches []memory.ContentMatch
	count   int64
	enabled bool
}

func (m *mockContentSearcher) Search(query, repo string) ([]memory.ContentMatch, error) {
	if !m.enabled {
		return nil, fmt.Errorf("content search is disabled")
	}
	return m.matches, nil
}

func (m *mockContentSearcher) Count() (int64, error) {
	return m.count, nil
}

type mockGraphStore struct {
	count int64
}

func (m *mockGraphStore) EntityCount() (int64, error) {
	return m.count, nil
}
func (m *mockGraphStore) CreateEntities(entities []memory.Entity) ([]memory.Entity, error) { return nil, nil }
func (m *mockGraphStore) CreateRelations(relations []memory.Relation) ([]memory.Relation, error) { return nil, nil }
func (m *mockGraphStore) AddObservations(observations []memory.Observation) ([]memory.Observation, error) { return nil, nil }
func (m *mockGraphStore) DeleteEntities(entityNames []string) error { return nil }
func (m *mockGraphStore) DeleteObservations(deletions []memory.Observation) error { return nil }
func (m *mockGraphStore) DeleteRelations(relations []memory.Relation) error { return nil }
func (m *mockGraphStore) ReadGraph() (memory.KnowledgeGraph, error) { return memory.KnowledgeGraph{}, nil }
func (m *mockGraphStore) SearchNodes(query string) (memory.KnowledgeGraph, error) { return memory.KnowledgeGraph{}, nil }
func (m *mockGraphStore) OpenNodes(names []string) (memory.KnowledgeGraph, error) { return memory.KnowledgeGraph{}, nil }
func (m *mockGraphStore) TraverseGraph(entity, relationType, direction string, maxDepth int) ([]memory.TraverseNode, error) { return nil, nil }

func TestGetScanStatusHandler(t *testing.T) {
	sc := &mockScanner{
		repos: []scanner.RepoInfo{
			{Name: "org/svc-a"},
			{Name: "org/svc-b"},
		},
	}
	counter := &mockGraphStore{count: 5}
	searcher := &mockContentSearcher{count: 10, enabled: true}

	handler := getScanStatusHandler(sc, counter, searcher)
	req := &mcp.CallToolRequest{}

	_, result, err := handler(t.Context(), req, ScanStatusArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RepoCount != 2 {
		t.Errorf("expected RepoCount=2, got %d", result.RepoCount)
	}
	if result.GraphEntities != 5 {
		t.Errorf("expected GraphEntities=5, got %d", result.GraphEntities)
	}
	if result.ContentIndexed != 10 {
		t.Errorf("expected ContentIndexed=10, got %d", result.ContentIndexed)
	}
	if !result.ContentEnabled {
		t.Error("expected ContentEnabled=true")
	}
}

func TestSearchContentHandler_Success(t *testing.T) {
	searcher := &mockContentSearcher{
		enabled: true,
		matches: []memory.ContentMatch{
			{RepoName: "org/payment-svc", Path: "README.md", Snippet: "...handles Stripe payments..."},
		},
	}

	handler := searchContentHandler(searcher, NewDocMetrics())
	req := &mcp.CallToolRequest{}

	_, result, err := handler(t.Context(), req, SearchContentArgs{Query: "stripe"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	if result.Matches[0].RepoName != "org/payment-svc" {
		t.Errorf("wrong repo: %s", result.Matches[0].RepoName)
	}
}

func TestSearchContentHandler_Disabled(t *testing.T) {
	searcher := &mockContentSearcher{enabled: false}
	handler := searchContentHandler(searcher, NewDocMetrics())
	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchContentArgs{Query: "anything"})
	if err == nil {
		t.Error("expected error when content search is disabled")
	}
}

func TestSearchContentHandler_EmptyQuery(t *testing.T) {
	searcher := &mockContentSearcher{enabled: true}
	handler := searchContentHandler(searcher, NewDocMetrics())
	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchContentArgs{Query: ""})
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestSearchDocsHandler_WhitespaceQuery(t *testing.T) {
	sc := &mockScanner{}
	handler := searchDocsHandler(sc)
	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchDocsArgs{Query: "   "})
	if err == nil {
		t.Error("expected error for whitespace-only query in search_docs")
	}
}

func TestSearchContentHandler_WhitespaceQuery(t *testing.T) {
	searcher := &mockContentSearcher{enabled: true}
	handler := searchContentHandler(searcher, NewDocMetrics())
	req := &mcp.CallToolRequest{}

	_, _, err := handler(t.Context(), req, SearchContentArgs{Query: "\t\n"})
	if err == nil {
		t.Error("expected error for whitespace-only query in search_content")
	}
}
