// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// ToolMetrics tracks call counts per MCP tool name in a thread-safe manner.

type ToolMetrics struct {
	mu sync.RWMutex

	counts map[string]*atomic.Int64
}

// NewToolMetrics returns a new, empty ToolMetrics instance.

func NewToolMetrics() *ToolMetrics {

	return &ToolMetrics{counts: make(map[string]*atomic.Int64)}

}

// Record increments the call counter for the given tool name.

func (m *ToolMetrics) Record(toolName string) {

	m.mu.RLock()

	c, ok := m.counts[toolName]

	m.mu.RUnlock()

	if !ok {

		m.mu.Lock()

		if c, ok = m.counts[toolName]; !ok {

			c = new(atomic.Int64)

			m.counts[toolName] = c

		}

		m.mu.Unlock()

	}

	c.Add(1)

}

// Snapshot returns a point-in-time copy of all tool call counts.

func (m *ToolMetrics) Snapshot() map[string]int64 {

	m.mu.RLock()

	defer m.mu.RUnlock()

	result := make(map[string]int64, len(m.counts))

	for name, c := range m.counts {

		result[name] = c.Load()

	}

	return result

}

// DocAccess holds access statistics for a single indexed document.

type DocAccess struct {
	Repo string `json:"repo"`

	Path string `json:"path"`

	Count int64 `json:"count"`
}

// DocMetrics tracks how many times each indexed document has been fetched or

// returned in search results, keyed by "repo\tpath".

type DocMetrics struct {
	mu sync.RWMutex

	counts map[string]*atomic.Int64
}

// NewDocMetrics returns a new, empty DocMetrics instance.

func NewDocMetrics() *DocMetrics {

	return &DocMetrics{counts: make(map[string]*atomic.Int64)}

}

// Record increments the access counter for a specific repo+path pair.

func (d *DocMetrics) Record(repo, path string) {

	key := repo + "\t" + path

	d.mu.RLock()

	c, ok := d.counts[key]

	d.mu.RUnlock()

	if !ok {

		d.mu.Lock()

		if c, ok = d.counts[key]; !ok {

			c = new(atomic.Int64)

			d.counts[key] = c

		}

		d.mu.Unlock()

	}

	c.Add(1)

}

// TopN returns up to n documents sorted by access count descending.

// Pass n <= 0 to return all tracked documents.

func (d *DocMetrics) TopN(n int) []DocAccess {

	d.mu.RLock()

	entries := make([]DocAccess, 0, len(d.counts))

	for key, c := range d.counts {

		repo, path, _ := splitKey(key)

		entries = append(entries, DocAccess{Repo: repo, Path: path, Count: c.Load()})

	}

	d.mu.RUnlock()

	slices.SortFunc(entries, func(a, b DocAccess) int {

		return cmp.Compare(b.Count, a.Count)

	})

	if n > 0 && n < len(entries) {

		return entries[:n]

	}

	return entries

}

// splitKey splits a "repo\tpath" key back into its components.

func splitKey(key string) (repo, path string, ok bool) {

	repo, path, ok = strings.Cut(key, "\t")

	return

}
