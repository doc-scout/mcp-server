// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// dbDocContent stores cached file content indexed by repo and path.
type dbDocContent struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	RepoName  string    `gorm:"index;uniqueIndex:idx_repo_path"`
	Path      string    `gorm:"uniqueIndex:idx_repo_path"`
	SHA       string
	Content   string    `gorm:"type:text"`
	IndexedAt time.Time
}

// ContentMatch is a search result from the content cache.
type ContentMatch struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	Snippet  string `json:"snippet"`
}

// ContentCache stores and searches raw file content indexed during scans.
type ContentCache struct {
	db      *gorm.DB
	enabled bool
	maxSize int
}

// NewContentCache creates a ContentCache.
// enabled=false disables all writes and returns errors on Search.
// maxSize is the maximum byte size of content to store (files larger are skipped).
func NewContentCache(db *gorm.DB, enabled bool, maxSize int) *ContentCache {
	return &ContentCache{db: db, enabled: enabled, maxSize: maxSize}
}

// NeedsUpdate returns true if the file is not cached or its SHA has changed.
// Always returns false when the cache is disabled.
func (cc *ContentCache) NeedsUpdate(repoName, path, sha string) bool {
	if !cc.enabled {
		return false
	}
	var existing dbDocContent
	err := cc.db.Where("repo_name = ? AND path = ?", repoName, path).First(&existing).Error
	if err != nil {
		return true // not found
	}
	return existing.SHA != sha
}

// Upsert stores or updates the content for a file.
// Files exceeding maxSize are silently skipped.
// No-ops when the cache is disabled.
func (cc *ContentCache) Upsert(repoName, path, sha, content string) error {
	if !cc.enabled {
		return nil
	}
	if len(content) > cc.maxSize {
		return nil // skip oversized files silently
	}
	row := dbDocContent{
		RepoName:  repoName,
		Path:      path,
		SHA:       sha,
		Content:   content,
		IndexedAt: time.Now(),
	}
	return cc.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repo_name"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"sha", "content", "indexed_at"}),
	}).Create(&row).Error
}

// Search performs a case-insensitive full-text search across cached content.
// Optionally filter by repoName (pass "" for no filter).
// Returns up to 20 results with a snippet of ~300 chars around the first match.
// Returns an error if the cache is disabled.
func (cc *ContentCache) Search(query, repoName string) ([]ContentMatch, error) {
	if !cc.enabled {
		return nil, fmt.Errorf("content search is disabled: set SCAN_CONTENT=true and restart with a persistent DATABASE_URL to enable it")
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query must not be empty or whitespace-only")
	}

	// Escape SQL LIKE special characters so the query is treated as a literal string.
	escaped := strings.ReplaceAll(query, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, "%", `\%`)
	escaped = strings.ReplaceAll(escaped, "_", `\_`)

	var rows []dbDocContent
	q := cc.db.Where("LOWER(content) LIKE LOWER(?) ESCAPE ?", "%"+escaped+"%", `\`)
	if repoName != "" {
		q = q.Where("repo_name = ?", repoName)
	}
	if err := q.Limit(20).Find(&rows).Error; err != nil {
		return nil, err
	}

	matches := make([]ContentMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, ContentMatch{
			RepoName: row.RepoName,
			Path:     row.Path,
			Snippet:  extractSnippet(row.Content, query, 300),
		})
	}
	return matches, nil
}

// Count returns the number of files currently in the content cache.
func (cc *ContentCache) Count() (int64, error) {
	var count int64
	err := cc.db.Model(&dbDocContent{}).Count(&count).Error
	return count, err
}

// DeleteOrphanedContent removes content rows for repos not in activeRepos.
func (cc *ContentCache) DeleteOrphanedContent(activeRepos []string) error {
	if !cc.enabled || len(activeRepos) == 0 {
		return nil
	}
	return cc.db.Where("repo_name NOT IN ?", activeRepos).Delete(&dbDocContent{}).Error
}

// extractSnippet returns ~snippetSize chars of context around the first occurrence of query.
func extractSnippet(content, query string, snippetSize int) string {
	lower := strings.ToLower(content)
	lowerQ := strings.ToLower(query)
	idx := strings.Index(lower, lowerQ)
	if idx < 0 {
		if len(content) > snippetSize {
			return content[:snippetSize] + "..."
		}
		return content
	}
	half := snippetSize / 2
	start := max(0, idx-half)
	end := min(len(content), idx+len(query)+half)
	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}
	return snippet
}
