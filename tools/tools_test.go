package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"docscout-mcp/scanner"
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
	req := mcp.CallToolRequest{}
	
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected IsError false")
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected content, got empty")
	}

	textContent, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent")
	}

	// Verify JSON unmarshals to the expected structure
	type repoSummary struct {
		Name        string   `json:"name"`
		FullName    string   `json:"full_name"`
		Description string   `json:"description"`
		URL         string   `json:"url"`
		FileCount   int      `json:"file_count"`
		FileTypes   []string `json:"file_types"`
	}

	var summaries []repoSummary
	if err := json.Unmarshal([]byte(textContent.Text), &summaries); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

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
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query": "guide",
	}

	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected IsError false")
	}

	textContent, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent")
	}

	var results []scanner.FileEntry
	if err := json.Unmarshal([]byte(textContent.Text), &results); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

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

	handler := getFileContentHandler(mock)
	
	// Test successful case
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"repo": "test-repo",
		"path": "docs/guide.md",
	}

	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected IsError false")
	}

	textContent, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent")
	}
	
	if textContent.Text != "# Guide\nThis is a guide." {
		t.Errorf("unexpected content: %s", textContent.Text)
	}

	// Test error case (file not indexed)
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]interface{}{
		"repo": "test-repo",
		"path": "docs/secret.txt",
	}

	res2, err := handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res2.IsError {
		t.Errorf("expected IsError true for unindexed file")
	}
}
