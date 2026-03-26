package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
	req := &mcp.CallToolRequest{}
	
	// The new SDK generic API signature
	res, output, err := handler(context.Background(), req, ListReposArgs{})
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

	res, output, err := handler(context.Background(), req, SearchDocsArgs{Query: "guide"})
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

	handler := getFileContentHandler(mock)
	
	// Test successful case
	req := &mcp.CallToolRequest{}

	res, output, err := handler(context.Background(), req, GetFileContentArgs{Repo: "test-repo", Path: "docs/guide.md"})
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

	_, _, err2 := handler(context.Background(), req2, GetFileContentArgs{Repo: "test-repo", Path: "docs/secret.txt"})
	
	// The new behavior returns a standard Go compilation error handled by the wrapper.
	if err2 == nil {
		t.Errorf("expected error for unindexed file")
	}
}
