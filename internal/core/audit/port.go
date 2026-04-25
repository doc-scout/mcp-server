// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package audit

import (
	"context"
	"time"
)

// AuditWriter is the write-only view used by the audit logger decorator.
type AuditWriter interface {
	Write(ctx context.Context, e AuditEvent) error
}

// AuditReader is the read-only view used by MCP tools and HTTP handlers.
type AuditReader interface {
	Query(ctx context.Context, f AuditFilter) ([]AuditEvent, int64, error)
	Summary(ctx context.Context, window time.Duration) (AuditSummary, error)
}

// AuditStore is the full port implemented by internal/infra/db.
type AuditStore interface {
	AuditWriter
	AuditReader
}
