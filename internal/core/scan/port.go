// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package scan

import (
	"context"
	"time"
)

// ScannerGateway is the outbound port implemented by internal/infra/github.
type ScannerGateway interface {
	Start(ctx context.Context)
	SetOnScanComplete(func([]RepoInfo))
	Status() (scanning bool, lastScan time.Time, repoCount int)
	TriggerScan() bool
	TriggerRepoScan(ctx context.Context, owner, repo string)
}

// DocumentService is the inbound port called by internal/adapter/mcp tools.
type DocumentService interface {
	ListRepos() []RepoInfo
	SearchDocs(query string) []FileEntry
	GetFileContent(ctx context.Context, repo, path string) (string, error)
	Status() (scanning bool, lastScan time.Time, repoCount int)
	TriggerScan() bool
}
