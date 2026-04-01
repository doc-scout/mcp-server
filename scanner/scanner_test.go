// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package scanner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v60/github"
)

func setupMockGitHub() (*httptest.Server, *github.Client) {
	mux := http.NewServeMux()

	// Mock list org repos: GET /orgs/test-org/repos
	mux.HandleFunc("/orgs/test-org/repos", func(w http.ResponseWriter, r *http.Request) {
		repos := []map[string]interface{}{
			{
				"id":        1,
				"name":      "repo1",
				"full_name": "test-org/repo1",
				"owner": map[string]interface{}{
					"login": "test-org",
				},
			},
			{
				"id":        2,
				"name":      "repo2",
				"full_name": "test-org/repo2",
				"owner": map[string]interface{}{
					"login": "test-org",
				},
			},
		}
		json.NewEncoder(w).Encode(repos)
	})

	// Mock file contents for repo1
	mux.HandleFunc("/repos/test-org/repo1/contents/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/test-org/repo1/contents/")

		if path == "README.md" {
			content := map[string]interface{}{
				"type":     "file",
				"path":     "README.md",
				"name":     "README.md",
				"content":  base64.StdEncoding.EncodeToString([]byte("# Test Repo 1\nDocs here.")),
				"encoding": "base64",
			}
			json.NewEncoder(w).Encode(content)
			return
		}

		if path == "docs" {
			// dir contents
			content := []map[string]interface{}{
				{
					"type": "file",
					"path": "docs/guide.md",
					"name": "guide.md",
				},
			}
			json.NewEncoder(w).Encode(content)
			return
		}

		if path == "docs/guide.md" {
			content := map[string]interface{}{
				"type":     "file",
				"path":     "docs/guide.md",
				"name":     "guide.md",
				"content":  base64.StdEncoding.EncodeToString([]byte("Guide content")),
				"encoding": "base64",
			}
			json.NewEncoder(w).Encode(content)
			return
		}

		// Fallback for missing files
		http.Error(w, "Not found", http.StatusNotFound)
	})

	// Mock repo2 as empty
	mux.HandleFunc("/repos/test-org/repo2/contents/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	})

	ts := httptest.NewServer(mux)

	client := github.NewClient(nil)
	client.BaseURL, _ = url.Parse(ts.URL + "/")
	// Important: To test endpoints correctly with httptest, we need to bypass github.Client's
	// validation or make sure our mock server's URL ends with a slash.

	return ts, client
}

func TestScanner_scanOrg(t *testing.T) {
	ts, client := setupMockGitHub()
	defer ts.Close()

	scanner := New(client, "test-org", 0, []string{"README.md"}, []string{"docs"}, nil, nil, nil, nil)
	scanner.scanOrg(context.Background())

	repos := scanner.ListRepos()
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo with docs, got %d", len(repos))
	}

	if repos[0].Name != "test-org/repo1" {
		t.Errorf("expected repo1, got %s", repos[0].Name)
	}

	if len(repos[0].Files) != 2 {
		t.Fatalf("expected 2 files (README.md, docs/guide.md), got %d", len(repos[0].Files))
	}
}

func TestScanner_GetFileContent(t *testing.T) {
	ts, client := setupMockGitHub()
	defer ts.Close()

	scanner := New(client, "test-org", 0, []string{"README.md"}, []string{"docs"}, nil, nil, nil, nil)
	scanner.scanOrg(context.Background())

	// Test a valid file
	content, err := scanner.GetFileContent(context.Background(), "test-org/repo1", "README.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "# Test Repo 1\nDocs here." {
		t.Errorf("expected correct markdown content, got %s", content)
	}

	// Test an unindexed file
	_, err = scanner.GetFileContent(context.Background(), "test-org/repo1", "secret.txt")
	if err == nil {
		t.Errorf("expected error for unindexed file")
	}
}

func TestScanner_OnScanComplete(t *testing.T) {
	ts, client := setupMockGitHub()
	defer ts.Close()

	s := New(client, "test-org", 0, []string{"README.md"}, []string{"docs"}, nil, nil, nil, nil)

	called := false
	var callbackRepos []RepoInfo
	s.SetOnScanComplete(func(repos []RepoInfo) {
		called = true
		callbackRepos = repos
	})

	s.scanOrg(context.Background())

	if !called {
		t.Fatal("OnScanComplete callback was not called")
	}
	if len(callbackRepos) != 1 {
		t.Errorf("expected 1 repo in callback, got %d", len(callbackRepos))
	}
}

func TestScanner_SearchDocs(t *testing.T) {
	ts, client := setupMockGitHub()
	defer ts.Close()

	scanner := New(client, "test-org", 0, []string{"README.md"}, []string{"docs"}, nil, nil, nil, nil)
	scanner.scanOrg(context.Background())

	results := scanner.SearchDocs("guide")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Path != "docs/guide.md" {
		t.Errorf("expected 'docs/guide.md', got %s", results[0].Path)
	}
}

func TestScanner_RepoScanRespectsContext(t *testing.T) {
	ts, client := setupMockGitHub()
	defer ts.Close()

	s := New(client, "test-org", 0, []string{"README.md"}, []string{"docs"}, nil, nil, nil, nil)

	// Run with a pre-cancelled context — scanOrg should return without blocking.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	done := make(chan struct{})
	go func() {
		s.scanOrg(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good: completed without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("scanOrg did not complete within 3 seconds with a cancelled context")
	}
}
