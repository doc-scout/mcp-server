package scanner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v60/github"
)

// DefaultTargetFiles defines the default filenames to look for at the repo root.
var DefaultTargetFiles = []string{
	"catalog-info.yaml",
	"mkdocs.yml",
	"openapi.yaml",
	"swagger.json",
	"README.md",
}

// DefaultScanDirs defines the default directories to scan recursively for .md files.
var DefaultScanDirs = []string{
	"docs",
}

// FileEntry represents an indexed documentation file.
type FileEntry struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Type     string `json:"type"` // e.g. "catalog-info", "mkdocs", "openapi", "swagger", "readme", "docs"
}

// RepoInfo holds metadata about a repository that contains documentation.
type RepoInfo struct {
	Name        string      `json:"name"`
	FullName    string      `json:"full_name"`
	Description string      `json:"description"`
	HTMLURL     string      `json:"html_url"`
	Files       []FileEntry `json:"files"`
}

// Scanner manages GitHub org scanning and caching.
type Scanner struct {
	client       *github.Client
	org          string
	scanInterval time.Duration
	targetFiles  []string // files to look for at repo root
	scanDirs     []string // directories to scan recursively for .md files

	mu    sync.RWMutex
	repos map[string]*RepoInfo // keyed by repo name

	scanning   bool
	lastScanAt time.Time
}

// New creates a new Scanner instance.
func New(client *github.Client, org string, scanInterval time.Duration, targetFiles, scanDirs []string) *Scanner {
	if len(targetFiles) == 0 {
		targetFiles = DefaultTargetFiles
	}
	if len(scanDirs) == 0 {
		scanDirs = DefaultScanDirs
	}
	return &Scanner{
		client:       client,
		org:          org,
		scanInterval: scanInterval,
		targetFiles:  targetFiles,
		scanDirs:     scanDirs,
		repos:        make(map[string]*RepoInfo),
	}
}

// Start begins the initial scan and schedules periodic re-scans.
func (s *Scanner) Start(ctx context.Context) {
	// Initial scan in a goroutine so it doesn't block server startup.
	go func() {
		log.Println("[scanner] Starting initial org scan...")
		s.scanOrg(ctx)
		log.Printf("[scanner] Initial scan complete. Found %d repos with docs.\n", len(s.repos))
	}()

	// Periodic re-scan.
	if s.scanInterval > 0 {
		go func() {
			ticker := time.NewTicker(s.scanInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					log.Println("[scanner] Starting periodic re-scan...")
					s.scanOrg(ctx)
					log.Printf("[scanner] Re-scan complete. Found %d repos with docs.\n", len(s.repos))
				}
			}
		}()
	}
}

// scanOrg lists all repos and scans each for target documentation files.
func (s *Scanner) scanOrg(ctx context.Context) {
	s.mu.Lock()
	s.scanning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.scanning = false
		s.lastScanAt = time.Now()
		s.mu.Unlock()
	}()

	allRepos, err := s.listAllRepos(ctx)
	if err != nil {
		log.Printf("[scanner] Error listing repos: %v\n", err)
		return
	}

	log.Printf("[scanner] Found %d total repos for %s\n", len(allRepos), s.org)

	// Use a semaphore to limit concurrent API calls.
	const maxConcurrency = 5
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	newRepos := make(map[string]*RepoInfo)
	var mu sync.Mutex

	for _, repo := range allRepos {
		repoName := repo.GetName()
		wg.Add(1)
		sem <- struct{}{} // acquire

		go func(repoName string, repo *github.Repository) {
			defer wg.Done()
			defer func() { <-sem }() // release

			files := s.scanRepo(ctx, repoName)
			if len(files) > 0 {
				info := &RepoInfo{
					Name:        repoName,
					FullName:    repo.GetFullName(),
					Description: repo.GetDescription(),
					HTMLURL:     repo.GetHTMLURL(),
					Files:       files,
				}
				mu.Lock()
				newRepos[repoName] = info
				mu.Unlock()
			}
		}(repoName, repo)
	}

	wg.Wait()

	// Swap entire cache atomically.
	s.mu.Lock()
	s.repos = newRepos
	s.mu.Unlock()
}

// listAllRepos paginates through all repos for the configured owner.
// It first tries the Organization endpoint; if the owner is a personal
// account (404), it transparently falls back to the User endpoint.
func (s *Scanner) listAllRepos(ctx context.Context) ([]*github.Repository, error) {
	repos, err := s.listByOrg(ctx)
	if err == nil {
		return repos, nil
	}

	// Check if the error is a 404 (owner is a user, not an org).
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == 404 {
		log.Printf("[scanner] '%s' is not an org, trying as user account...\n", s.org)
		return s.listByUser(ctx)
	}

	return nil, err
}

func (s *Scanner) listByOrg(ctx context.Context) ([]*github.Repository, error) {
	var all []*github.Repository
	opts := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		repos, resp, err := s.client.Repositories.ListByOrg(ctx, s.org, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

func (s *Scanner) listByUser(ctx context.Context) ([]*github.Repository, error) {
	var all []*github.Repository
	opts := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		repos, resp, err := s.client.Repositories.ListByUser(ctx, s.org, opts)
		if err != nil {
			return nil, fmt.Errorf("listing user repos page %d: %w", opts.Page, err)
		}
		all = append(all, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// scanRepo checks a single repo for target documentation files.
func (s *Scanner) scanRepo(ctx context.Context, repoName string) []FileEntry {
	var entries []FileEntry

	// Check root-level target files.
	for _, target := range s.targetFiles {
		fc, _, resp, err := s.client.Repositories.GetContents(ctx, s.org, repoName, target, nil)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				continue
			}
			log.Printf("[scanner] Error checking %s/%s: %v\n", repoName, target, err)
			continue
		}
		if fc != nil {
			entries = append(entries, FileEntry{
				RepoName: repoName,
				Path:     target,
				SHA:      fc.GetSHA(),
				Type:     classifyFile(target),
			})
		}
	}

	// Check configured directories recursively for .md files.
	for _, dir := range s.scanDirs {
		dirEntries := s.scanDocsDir(ctx, repoName, dir)
		entries = append(entries, dirEntries...)
	}

	return entries
}

// scanDocsDir recursively scans a directory for .md files.
func (s *Scanner) scanDocsDir(ctx context.Context, repoName, path string) []FileEntry {
	var entries []FileEntry

	_, dirContents, resp, err := s.client.Repositories.GetContents(ctx, s.org, repoName, path, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil
		}
		log.Printf("[scanner] Error scanning docs dir %s/%s: %v\n", repoName, path, err)
		return nil
	}

	for _, item := range dirContents {
		itemPath := item.GetPath()
		switch item.GetType() {
		case "file":
			if strings.HasSuffix(strings.ToLower(itemPath), ".md") {
				entries = append(entries, FileEntry{
					RepoName: repoName,
					Path:     itemPath,
					SHA:      item.GetSHA(),
					Type:     "docs",
				})
			}
		case "dir":
			// Recurse into subdirectories.
			subEntries := s.scanDocsDir(ctx, repoName, itemPath)
			entries = append(entries, subEntries...)
		}
	}

	return entries
}

// classifyFile returns a type label for a given filename.
func classifyFile(name string) string {
	base := filepath.Base(strings.ToLower(name))
	switch base {
	case "catalog-info.yaml":
		return "catalog-info"
	case "mkdocs.yml":
		return "mkdocs"
	case "openapi.yaml":
		return "openapi"
	case "swagger.json":
		return "swagger"
	case "readme.md":
		return "readme"
	default:
		return "docs"
	}
}

// --- Public API used by MCP tools ---

// ListRepos returns all repos that have documentation files.
func (s *Scanner) ListRepos() []RepoInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]RepoInfo, 0, len(s.repos))
	for _, info := range s.repos {
		result = append(result, *info)
	}
	return result
}

// SearchDocs searches file paths and repo names for the given query term.
func (s *Scanner) SearchDocs(query string) []FileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := strings.ToLower(query)
	var results []FileEntry

	for _, info := range s.repos {
		for _, f := range info.Files {
			if strings.Contains(strings.ToLower(f.Path), q) ||
				strings.Contains(strings.ToLower(f.RepoName), q) {
				results = append(results, f)
			}
		}
	}
	return results
}

// IsIndexed checks if a specific file path is indexed in the cache as documentation.
func (s *Scanner) IsIndexed(repoName, path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, ok := s.repos[repoName]
	if !ok {
		return false
	}

	for _, f := range repo.Files {
		if strings.EqualFold(f.Path, path) {
			return true
		}
	}
	return false
}

// GetFileContent retrieves the raw content of a file from GitHub.
// For security (preventing path traversal to non-docs), it enforces that the file must be indexed.
func (s *Scanner) GetFileContent(ctx context.Context, repoName, path string) (string, error) {
	if !s.IsIndexed(repoName, path) {
		return "", fmt.Errorf("security policy: path '%s' is not indexed as a documentation file", path)
	}

	fc, _, resp, err := s.client.Repositories.GetContents(ctx, s.org, repoName, path, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return "", fmt.Errorf("file not found: %s/%s", repoName, path)
		}
		return "", fmt.Errorf("error fetching file %s/%s: %w", repoName, path, err)
	}
	if fc == nil {
		return "", fmt.Errorf("path is a directory, not a file: %s/%s", repoName, path)
	}

	content, err := fc.GetContent()
	if err != nil {
		return "", fmt.Errorf("error decoding content of %s/%s: %w", repoName, path, err)
	}
	return content, nil
}

// Status returns scanning status info.
func (s *Scanner) Status() (scanning bool, lastScan time.Time, repoCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scanning, s.lastScanAt, len(s.repos)
}
