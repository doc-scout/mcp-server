// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package scan

// FileEntry represents an indexed documentation file.
type FileEntry struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Type     string `json:"type"`
}

// RepoInfo holds metadata about a repository that contains documentation.
type RepoInfo struct {
	Name        string      `json:"name"`
	FullName    string      `json:"full_name"`
	Description string      `json:"description"`
	HTMLURL     string      `json:"html_url"`
	Files       []FileEntry `json:"files"`
}
