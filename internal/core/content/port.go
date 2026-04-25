// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package content

// ContentSearcher is the inbound port called by internal/adapter/mcp tools.
type ContentSearcher interface {
	Search(query, repo, fileType string) ([]ContentMatch, error)
	Count() (int64, error)
	SearchMode() string
}

// ContentRepository is the outbound port implemented by internal/infra/db.
type ContentRepository interface {
	ContentSearcher
	Upsert(repoName, path, sha, content, fileType string) error
	NeedsUpdate(repoName, path, sha string) bool
	DeleteOrphanedContent(activeRepos []string) error
	ListDocs(repoName string) ([]DocRecord, error)
}
