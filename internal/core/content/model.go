// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package content

// ContentMatch is a search result from the content cache.
type ContentMatch struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	FileType string `json:"file_type,omitempty"`
	Snippet  string `json:"snippet"`
}

// DocRecord is a lightweight document record used by the semantic indexer.
type DocRecord struct {
	RepoName string
	Path     string
	DocID    string // "<RepoName>#<Path>"
	Content  string
}
