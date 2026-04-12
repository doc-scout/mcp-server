// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"fmt"
	"strings"
	"testing"
)

func newTestContentCache(t *testing.T, maxSize int) *ContentCache {
	t.Helper()
	n := testCounter.Add(1)
	dsn := fmt.Sprintf("file:contentdb_%d?mode=memory&cache=shared", n)
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	return NewContentCache(db, true, maxSize)
}

func TestContentCache_Upsert_New(t *testing.T) {
	cache := newTestContentCache(t, 1024)

	err := cache.Upsert("my-org/svc-a", "README.md", "sha1", "# Service A\nThis handles payments.", "readme")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	count, err := cache.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cached file, got %d", count)
	}
}

func TestContentCache_Upsert_SkipsSameSHA(t *testing.T) {
	cache := newTestContentCache(t, 1024)

	cache.Upsert("my-org/svc-a", "README.md", "sha1", "original content", "readme")
	// Second upsert with same SHA should be a no-op (NeedsUpdate returns false)
	if cache.NeedsUpdate("my-org/svc-a", "README.md", "sha1") {
		t.Error("NeedsUpdate should be false for same SHA")
	}
}

func TestContentCache_Upsert_UpdatesOnNewSHA(t *testing.T) {
	cache := newTestContentCache(t, 1024)

	cache.Upsert("my-org/svc-a", "README.md", "sha1", "old content", "readme")
	if !cache.NeedsUpdate("my-org/svc-a", "README.md", "sha2") {
		t.Error("NeedsUpdate should be true when SHA changes")
	}
	cache.Upsert("my-org/svc-a", "README.md", "sha2", "new content", "readme")

	// Verify stored SHA is now sha2
	if cache.NeedsUpdate("my-org/svc-a", "README.md", "sha2") {
		t.Error("NeedsUpdate should be false after updating to sha2")
	}
}

func TestContentCache_Upsert_SizeCap(t *testing.T) {
	// maxSize of 10 bytes — any real content exceeds it
	cache := newTestContentCache(t, 10)

	err := cache.Upsert("my-org/svc-a", "README.md", "sha1", "this content is definitely longer than ten bytes", "readme")
	if err != nil {
		t.Fatalf("Upsert with large content: %v", err)
	}

	count, _ := cache.Count()
	if count != 0 {
		t.Errorf("oversized file should not be cached, count=%d", count)
	}
}

func TestContentCache_Search_Basic(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/payment-svc", "README.md", "sha1", "# Payment Service\nHandles Stripe transactions and refunds.", "readme")
	cache.Upsert("org/auth-svc", "README.md", "sha2", "# Auth Service\nManages JWT tokens and sessions.", "readme")

	matches, err := cache.Search("stripe", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].RepoName != "org/payment-svc" {
		t.Errorf("wrong repo: %s", matches[0].RepoName)
	}
	if !strings.Contains(matches[0].Snippet, "Stripe") {
		t.Errorf("snippet should contain 'Stripe', got: %s", matches[0].Snippet)
	}
}

func TestContentCache_Search_FilterByRepo(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc-a", "README.md", "sha1", "payment processing logic", "readme")
	cache.Upsert("org/svc-b", "README.md", "sha2", "payment gateway integration", "readme")

	matches, err := cache.Search("payment", "org/svc-a", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match with repo filter, got %d", len(matches))
	}
	if matches[0].RepoName != "org/svc-a" {
		t.Errorf("wrong repo: %s", matches[0].RepoName)
	}
}

func TestContentCache_Search_CaseInsensitive(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)
	cache.Upsert("org/svc", "docs/api.md", "sha1", "The PAYMENT endpoint accepts POST requests.", "readme")

	matches, _ := cache.Search("payment", "", "")
	if len(matches) != 1 {
		t.Fatalf("expected case-insensitive match, got %d matches", len(matches))
	}
}

func TestContentCache_DeleteOrphanedContent(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/active-svc", "README.md", "sha1", "active service content", "readme")
	cache.Upsert("org/gone-svc", "README.md", "sha2", "removed service content", "readme")

	err := cache.DeleteOrphanedContent([]string{"org/active-svc"})
	if err != nil {
		t.Fatalf("DeleteOrphanedContent: %v", err)
	}

	count, _ := cache.Count()
	if count != 1 {
		t.Errorf("expected 1 remaining, got %d", count)
	}
}

func TestContentCache_Disabled(t *testing.T) {
	n := testCounter.Add(1)
	dsn := fmt.Sprintf("file:contentdb_disabled_%d?mode=memory&cache=shared", n)
	db, _ := OpenDB(dsn)
	cache := NewContentCache(db, false, 1024)

	err := cache.Upsert("org/svc", "README.md", "sha1", "content", "readme")
	if err != nil {
		t.Fatalf("Upsert on disabled cache should not error: %v", err)
	}

	count, _ := cache.Count()
	if count != 0 {
		t.Errorf("disabled cache should store nothing, count=%d", count)
	}

	_, err = cache.Search("anything", "", "")
	if err == nil {
		t.Error("Search on disabled cache should return error")
	}
}

func TestContentCache_Search_WildcardInQuery(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc", "README.md", "sha1", "discount: 50% off all items", "readme")
	cache.Upsert("org/svc2", "README.md", "sha2", "discount: anything off all items", "readme")

	// A literal "50%" should only match files that contain the literal string "50%".
	matches, err := cache.Search("50%", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("expected exactly 1 match for literal '50%%', got %d", len(matches))
	}
}

func TestContentCache_Search_WhitespaceOnlyQuery(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)
	cache.Upsert("org/svc", "README.md", "sha1", "some content", "readme")

	_, err := cache.Search("   ", "", "")
	if err == nil {
		t.Error("expected error for whitespace-only query")
	}
}

func TestContentCache_FTS5_MultiWordQuery(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc-a", "README.md", "sha1", "The payment service handles Stripe transactions and refunds.", "readme")
	cache.Upsert("org/svc-b", "README.md", "sha2", "Auth service manages JWT tokens.", "readme")
	cache.Upsert("org/svc-c", "README.md", "sha3", "Stripe integration for subscription billing.", "readme")

	// Multi-word query: both words must appear somewhere in the content.
	matches, err := cache.Search("stripe payment", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match for 'stripe payment'")
	}
	// svc-a has both "stripe" and "payment" — must be in the results.
	found := false
	for _, m := range matches {
		if m.RepoName == "org/svc-a" {
			found = true
		}
	}
	if !found {
		t.Errorf("org/svc-a should match 'stripe payment', got: %v", matches)
	}
}

func TestContentCache_FTS5_Stemming(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc", "README.md", "sha1", "The authentication service manages tokens.", "readme")

	// "authenticate" should match "authentication" via Porter stemmer (both stem to "authent").
	matches, err := cache.Search("authenticate", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Errorf("expected stemmed match for 'authenticate' → 'authentication', got none")
	}
}

func TestContentCache_FTS5_RankedResults(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	// svc-a mentions "authentication" once; svc-b is entirely about authentication.
	cache.Upsert("org/svc-a", "README.md", "sha1", "This service handles payments. Authentication is handled elsewhere.", "readme")
	cache.Upsert("org/svc-b", "README.md", "sha2", "Authentication service. Manages authentication tokens. Provides authentication middleware.", "readme")

	matches, err := cache.Search("authentication", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	// svc-b mentions authentication 3× — BM25 should rank it first.
	if matches[0].RepoName != "org/svc-b" {
		t.Errorf("expected org/svc-b (higher frequency) ranked first, got %s", matches[0].RepoName)
	}
}

func TestContentCache_FTS5_SnippetNotEmpty(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)
	cache.Upsert("org/svc", "README.md", "sha1", "The fraud detection service analyses transaction patterns to identify suspicious activity.", "readme")

	matches, err := cache.Search("fraud", "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected match")
	}
	if matches[0].Snippet == "" {
		t.Error("snippet should not be empty")
	}
}

func TestContentCache_Search_FilterByFileType(t *testing.T) {
	cache := newTestContentCache(t, 1024*1024)

	cache.Upsert("org/svc", "README.md", "sha1", "payment processing logic", "readme")
	cache.Upsert("org/svc", "openapi.yaml", "sha2", "payment gateway API definition", "openapi")

	// Filter to only readme: should miss the openapi file.
	matches, err := cache.Search("payment", "", "readme")
	if err != nil {
		t.Fatalf("Search with file_type filter: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 readme match, got %d", len(matches))
	}
	if matches[0].FileType != "readme" {
		t.Errorf("expected file_type 'readme', got '%s'", matches[0].FileType)
	}

	// No filter: both files match.
	all, err := cache.Search("payment", "", "")
	if err != nil {
		t.Fatalf("Search without file_type filter: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 matches without filter, got %d", len(all))
	}
}
