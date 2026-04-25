// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package audit

import "time"

// AuditEvent is persisted to the audit_events table.
type AuditEvent struct {
	ID        string    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	Agent     string
	Tool      string
	Operation string
	Targets   string // JSON array of entity/relation names
	Count     int
	Outcome   string // "ok" | "error"
	ErrorMsg  string
}

// AuditFilter constrains Query results.
type AuditFilter struct {
	Agent     string
	Tool      string
	Operation string
	Outcome   string
	Since     time.Time
	Limit     int
}

// AuditSummary is returned by AuditReader.Summary.
type AuditSummary struct {
	TotalMutations int
	ByAgent        map[string]int
	ByOperation    map[string]int
	ErrorRate      float64
	RiskyEvents    []AuditEvent
}
