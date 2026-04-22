# Auditing — Governance & Discovery Design

**Date:** 2026-04-21
**Status:** Approved

---

## Overview

Add persistent, agent-aware audit logging to DocScout-MCP so that governance reviewers and LLM agents can query what changed, when, and by whom. Builds on the existing `GraphAuditLogger` decorator without changing any tool handler signatures.

Two capabilities:

- **B — Agent identity tracking**: every graph mutation is tagged with the identity of the agent that triggered it.
- **C — Audit report tool**: surfaces anomalies (mass deletes, error bursts, unknown agents) via a queryable MCP tool and HTTP endpoint.

---

## Identity Resolution

Agent identity is resolved once at server startup and injected into `GraphAuditLogger`:

| Priority | Source | Example |
|---|---|---|
| 1 | `AGENT_ID` env var | `"indexer-bot"` |
| 2 | `clientInfo.name` from MCP `initialize` handshake | `"claude-desktop"` |
| 3 | Fallback | `"unknown"` |

`main.go` intercepts `clientInfo.name` in the MCP initialize handler and passes the resolved string to `NewGraphAuditLogger`. No changes to tool handler signatures.

---

## Data Model

New `audit_events` GORM table, auto-migrated alongside existing tables:

```go
type AuditEvent struct {
    ID        string    // PK, UUIDv7 (time-sortable — ORDER BY id = chronological)
    CreatedAt time.Time // human-readable timestamp
    Agent     string    // resolved agent identity
    Tool      string    // MCP tool name, e.g. "create_entities"
    Operation string    // "create" | "delete" | "update" | "add"
    Targets   string    // JSON array of affected entity/relation names
    Count     int       // number of items mutated
    Outcome   string    // "ok" | "error"
    ErrorMsg  string    // populated on failure, empty otherwise
}
```

UUIDv7 is generated via `github.com/google/uuid` v1.6.0 (already vendored). No `created_at` index needed — UUIDv7 ordering is chronological.

**Persistence gate**: the table is created only when `DATABASE_URL` points to a persistent store. When running in-memory, `AuditStore` is nil and all audit writes are silent no-ops. Existing slog mutation lines are preserved in both modes.

---

## GraphAuditLogger Changes

`GraphAuditLogger` gains two fields:

- `agent string` — resolved at construction, immutable
- `store AuditStore` — optional DB writer; nil = no-op

After each mutation (success or failure), the decorator writes one `AuditEvent` row via `store`. Write failures are logged to slog but never propagate — a failed audit write must never fail the underlying graph mutation.

---

## MCP Tools

### `query_audit_log`

Retrieves raw audit events with optional filters.

```
Args:
  agent      string   optional — filter by agent name
  tool       string   optional — filter by tool name
  operation  string   optional — "create"|"delete"|"update"|"add"
  outcome    string   optional — "ok"|"error"
  since      string   optional — RFC3339 timestamp lower bound
  limit      int      optional — default 50, max 500

Result:
  events  []AuditEvent
  total   int          total matching rows
```

### `get_audit_summary`

Anomaly-focused report over a rolling time window.

```
Args:
  window  string  optional — "1h"|"24h"|"7d", default "24h"

Result:
  total_mutations  int
  by_agent         map[string]int
  by_operation     map[string]int
  error_rate       float64
  risky_events     []AuditEvent
```

**Risky event criteria:**
- Any `delete` operation with `count > 10`
- Any `outcome = "error"` burst: > 5 errors from the same agent within 1 hour
- Any event where `agent = "unknown"`

**When persistence is disabled**: both tools return a structured error message: `"audit persistence not enabled — set DATABASE_URL to a persistent store"`.

---

## HTTP Endpoints

Protected by the existing Bearer token middleware.

### `GET /audit`

Mirrors `query_audit_log`. Filter params as query strings:

```
GET /audit?agent=indexer-bot&operation=delete&since=2026-04-20T00:00:00Z&limit=100
→ 200 JSON: { "events": [...], "total": N }
→ 503 JSON: { "error": "audit persistence not enabled..." }  (when DATABASE_URL absent)
```

### `GET /audit/summary`

Mirrors `get_audit_summary`:

```
GET /audit/summary?window=24h
→ 200 JSON: { "total_mutations": N, "by_agent": {...}, "by_operation": {...}, "error_rate": 0.02, "risky_events": [...] }
→ 503 JSON: { "error": "audit persistence not enabled..." }
```

---

## Error Handling & Degradation

| Scenario | Behavior |
|---|---|
| No `DATABASE_URL` | `AuditStore` is nil; writes are no-ops; slog lines preserved |
| Audit write fails | Logged to slog; never propagates to caller; mutation result returned normally |
| `query_audit_log` with no results | `{ "events": [], "total": 0 }` — not an error |
| `agent = "unknown"` | Stored and queryable; flagged as risky in summary |

---

## Testing

### Unit (`tools/audit_store_test.go`)
- Write + query round-trip with all filter combinations
- UUIDv7 ordering matches insertion order
- Nil `AuditStore` is a clean no-op (no panic, no log noise)

### Integration (`tests/audit/audit_test.go`)
- `create_entities` → `query_audit_log` returns event with correct agent, tool, count
- `delete_entities` count > 10 → appears in `get_audit_summary` risky_events
- Error outcome recorded when mutation fails
- `/audit` and `/audit/summary` return correct JSON with expected fields

Follows the same E2E harness pattern as `tests/traverse_graph/` and `tests/integration_map/`.

---

## Files Affected

| File | Change |
|---|---|
| `memory/audit_store.go` | New — GORM model + `AuditStore` interface + SQLite/Postgres impl |
| `tools/audit.go` | Add `agent`, `store` fields; write `AuditEvent` after each mutation |
| `tools/query_audit_log.go` | New — MCP tool handler |
| `tools/get_audit_summary.go` | New — MCP tool handler |
| `tools/tools.go` | Register two new tools |
| `tools/ports.go` | Add `AuditStore` interface |
| `main.go` | Resolve agent identity; wire `AuditStore`; register `/audit` and `/audit/summary` routes |
| `tools/audit_store_test.go` | New — unit tests |
| `tests/audit/audit_test.go` | New — integration tests |
