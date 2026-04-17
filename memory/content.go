// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// dbDocContent stores cached file content indexed by repo and path.

type dbDocContent struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	RepoName string `gorm:"index;uniqueIndex:idx_repo_path"`

	Path string `gorm:"uniqueIndex:idx_repo_path"`

	SHA string

	FileType string `gorm:"index"` // e.g. "readme", "docs", "openapi", "catalog"

	Content string `gorm:"type:text"`

	IndexedAt time.Time
}

// ContentMatch is a search result from the content cache.

type ContentMatch struct {
	RepoName string `json:"repo_name"`

	Path string `json:"path"`

	FileType string `json:"file_type,omitempty"`

	Snippet string `json:"snippet"`
}

// DocRecord is a lightweight document record used by the semantic indexer.

type DocRecord struct {
	RepoName string

	Path string

	DocID string // "<RepoName>#<Path>"

	Content string
}

// ContentCache stores and searches raw file content indexed during scans.

type ContentCache struct {
	db *gorm.DB

	enabled bool

	maxSize int

	useFTS5 bool // true when the underlying DB is SQLite and FTS5 was initialised

}

// NewContentCache creates a ContentCache.

// enabled=false disables all writes and returns errors on Search.

// maxSize is the maximum byte size of content to store (files larger are skipped).

// FTS5 full-text search is automatically enabled when db is a SQLite connection.

func NewContentCache(db *gorm.DB, enabled bool, maxSize int) *ContentCache {

	cc := &ContentCache{db: db, enabled: enabled, maxSize: maxSize}

	if enabled && db.Name() == "sqlite" {

		if err := cc.initFTS5(); err != nil {

			slog.Warn("[content] FTS5 setup failed, falling back to LIKE search", "error", err)

		} else {

			cc.useFTS5 = true

			slog.Info("[content] FTS5 full-text search enabled")

		}

	}

	return cc

}

// initFTS5 creates the FTS5 virtual table and the three sync triggers on db_doc_contents.

// Safe to call multiple times — all statements use IF NOT EXISTS.

func (cc *ContentCache) initFTS5() error {

	stmts := []string{

		// FTS5 virtual table: repo_name stored but not tokenised (UNINDEXED),

		// content tokenised with Porter stemmer + ASCII case-folding.

		`CREATE VIRTUAL TABLE IF NOT EXISTS doc_contents_fts USING fts5(

			repo_name UNINDEXED,

			content,

			tokenize = 'porter ascii'

		)`,

		// Keep FTS index in sync with the main table.

		`CREATE TRIGGER IF NOT EXISTS doc_contents_fts_insert

		 AFTER INSERT ON db_doc_contents BEGIN

		   INSERT INTO doc_contents_fts(rowid, repo_name, content)

		   VALUES (new.id, new.repo_name, new.content);

		 END`,

		`CREATE TRIGGER IF NOT EXISTS doc_contents_fts_update

		 AFTER UPDATE ON db_doc_contents BEGIN

		   DELETE FROM doc_contents_fts WHERE rowid = old.id;

		   INSERT INTO doc_contents_fts(rowid, repo_name, content)

		   VALUES (new.id, new.repo_name, new.content);

		 END`,

		`CREATE TRIGGER IF NOT EXISTS doc_contents_fts_delete

		 AFTER DELETE ON db_doc_contents BEGIN

		   DELETE FROM doc_contents_fts WHERE rowid = old.id;

		 END`,
	}

	for _, stmt := range stmts {

		if err := cc.db.Exec(stmt).Error; err != nil {

			return fmt.Errorf("initFTS5: %w", err)

		}

	}

	return nil

}

// NeedsUpdate returns true if the file is not cached or its SHA has changed.

// Always returns false when the cache is disabled.

func (cc *ContentCache) NeedsUpdate(repoName, path, sha string) bool { //nolint:unparam

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

// fileType is the classification key (e.g. "readme", "openapi") — stored for filtering.

// Files exceeding maxSize are silently skipped.

// No-ops when the cache is disabled.

func (cc *ContentCache) Upsert(repoName, path, sha, content, fileType string) error {

	if !cc.enabled {

		return nil

	}

	if len(content) > cc.maxSize {

		slog.Debug("[content] Skipping oversized file", "repo", repoName, "path", path, "size", len(content), "max", cc.maxSize)

		return nil

	}

	row := dbDocContent{

		RepoName: repoName,

		Path: path,

		SHA: sha,

		FileType: fileType,

		Content: content,

		IndexedAt: time.Now(),
	}

	return cc.db.Clauses(clause.OnConflict{

		Columns: []clause.Column{{Name: "repo_name"}, {Name: "path"}},

		DoUpdates: clause.AssignmentColumns([]string{"sha", "file_type", "content", "indexed_at"}),
	}).Create(&row).Error

}

// Search performs a full-text search across cached content.

// On SQLite: uses FTS5 with BM25 ranking and Porter stemming.

// On PostgreSQL: uses escaped LIKE (case-insensitive).

// Pass "" for repoName or fileType to skip those filters.

// Returns up to 20 results with a snippet of context around the first match.

// Returns an error if the cache is disabled.

func (cc *ContentCache) Search(query, repoName, fileType string) ([]ContentMatch, error) {

	if !cc.enabled {

		return nil, fmt.Errorf("content search is disabled: set SCAN_CONTENT=true and restart with a persistent DATABASE_URL to enable it")

	}

	if strings.TrimSpace(query) == "" {

		return nil, fmt.Errorf("query must not be empty or whitespace-only")

	}

	if cc.useFTS5 {

		return cc.searchFTS5(query, repoName, fileType)

	}

	return cc.searchLIKE(query, repoName, fileType)

}

// sanitizeFTS5Query converts a free-text query into a safe FTS5 MATCH expression.

// Each whitespace-delimited term is wrapped in double-quotes (FTS5 phrase syntax),

// which prevents MATCH syntax errors from special characters (%, *, " etc.).

// Terms are joined with spaces — FTS5 implicit AND — so all terms must be present.

func sanitizeFTS5Query(query string) string {

	terms := strings.Fields(query)

	quoted := make([]string, 0, len(terms))

	for _, t := range terms {

		// Escape any embedded double-quote by doubling it.

		t = strings.ReplaceAll(t, `"`, `""`)

		quoted = append(quoted, `"`+t+`"`)

	}

	return strings.Join(quoted, " ")

}

// searchFTS5 performs a ranked FTS5 full-text search (SQLite only).

func (cc *ContentCache) searchFTS5(query, repoName, fileType string) ([]ContentMatch, error) {

	// FTS5 snippet(): args are (table, column_index, match_start, match_end, ellipsis, tokens_per_fragment)

	// column_index 1 = content column.

	sql := `

		SELECT dc.repo_name, dc.path, dc.file_type,

		       snippet(doc_contents_fts, 1, '', '', '...', 48) AS snippet

		FROM doc_contents_fts

		JOIN db_doc_contents dc ON dc.id = doc_contents_fts.rowid

		WHERE doc_contents_fts MATCH ?`

	args := []any{sanitizeFTS5Query(query)}

	if repoName != "" {

		sql += " AND dc.repo_name = ?"

		args = append(args, repoName) //nolint:makezero

	}

	if fileType != "" {

		sql += " AND dc.file_type = ?"

		args = append(args, fileType) //nolint:makezero

	}

	sql += " ORDER BY rank LIMIT 20"

	type row struct {
		RepoName string

		Path string

		FileType string

		Snippet string
	}

	var rows []row

	if err := cc.db.Raw(sql, args...).Scan(&rows).Error; err != nil {

		// FTS5 MATCH syntax errors surface here — return a clear message.

		return nil, fmt.Errorf("FTS5 search failed: %w", err)

	}

	matches := make([]ContentMatch, 0, len(rows))

	for _, r := range rows {

		matches = append(matches, ContentMatch(r))

	}

	return matches, nil

}

// searchLIKE performs a case-insensitive LIKE search (PostgreSQL fallback).

func (cc *ContentCache) searchLIKE(query, repoName, fileType string) ([]ContentMatch, error) {

	// Escape SQL LIKE special characters so the query is treated as a literal string.

	escaped := strings.ReplaceAll(query, `\`, `\\`)

	escaped = strings.ReplaceAll(escaped, "%", `\%`)

	escaped = strings.ReplaceAll(escaped, "_", `\_`)

	var rows []dbDocContent

	q := cc.db.Where("LOWER(content) LIKE LOWER(?) ESCAPE ?", "%"+escaped+"%", `\`)

	if repoName != "" {

		q = q.Where("repo_name = ?", repoName)

	}

	if fileType != "" {

		q = q.Where("file_type = ?", fileType)

	}

	if err := q.Limit(20).Find(&rows).Error; err != nil {

		return nil, err

	}

	matches := make([]ContentMatch, 0, len(rows))

	for _, row := range rows {

		matches = append(matches, ContentMatch{

			RepoName: row.RepoName,

			Path: row.Path,

			FileType: row.FileType,

			Snippet: extractSnippet(row.Content, query, 300),
		})

	}

	return matches, nil

}

// SearchMode returns "fts5" when SQLite FTS5 is active, "like" otherwise.

func (cc *ContentCache) SearchMode() string {

	if cc.useFTS5 {

		return "fts5"

	}

	return "like"

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

// ListDocs returns document records for semantic indexing.

// If repoName is non-empty, only documents from that repository are returned.

// Passing "" returns all documents across all repos.

func (cc *ContentCache) ListDocs(repoName string) ([]DocRecord, error) {

	db := cc.db

	if repoName != "" {

		db = db.Where("repo_name = ?", repoName)

	}

	var rows []dbDocContent

	if err := db.Find(&rows).Error; err != nil {

		return nil, err

	}

	out := make([]DocRecord, len(rows))

	for i, r := range rows {

		out[i] = DocRecord{

			RepoName: r.RepoName,

			Path: r.Path,

			DocID: r.RepoName + "#" + r.Path,

			Content: r.Content,
		}

	}

	return out, nil

}
