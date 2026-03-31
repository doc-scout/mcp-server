// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"sync"
	"sync/atomic"
)

// ToolMetrics tracks call counts per MCP tool name in a thread-safe manner.
type ToolMetrics struct {
	mu     sync.RWMutex
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
