# updateEntity DB Error Swallowing Fix

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix a bug in `updateEntity` where a DB error during the duplicate-name check is silently swallowed, allowing a rename to proceed incorrectly.

**Architecture:** Replace the `gorm.First`-based duplicate check (which returns `ErrRecordNotFound` on miss, making error classification necessary) with a `COUNT(*)`-based check (which returns `0` on miss and a real error only on DB failure). This matches the existing pattern used throughout the same file.

**Tech Stack:** Go, GORM, SQLite/PostgreSQL (both supported transparently).

---

## Problem

In `memory/memory.go`, `store.updateEntity` checks whether the new name is already taken:

```go
var existing dbEntity
if err := tx.Where("name = ?", newName).First(&existing).Error; err == nil {
    return fmt.Errorf("entity %q already exists", newName)
}
```

`gorm.First` returns `gorm.ErrRecordNotFound` when no row matches. The code only rejects on `err == nil` (record found). Any other error — connection failure, constraint error, context cancellation — falls through silently, and the rename continues as if the name were free. This can produce duplicate primary keys or corrupt the graph.

## Fix

Replace with a `COUNT(*)` check — the same pattern used by `createEntities`, `createRelations`, and `addObservations` elsewhere in the file:

```go
var taken int64
if err := tx.Model(&dbEntity{}).Where("name = ?", newName).Count(&taken).Error; err != nil {
    return fmt.Errorf("checking entity name %q: %w", newName, err)
}
if taken > 0 {
    return fmt.Errorf("entity %q already exists", newName)
}
```

`COUNT(*)` never returns `ErrRecordNotFound`. The two outcomes are unambiguous:
- `err != nil` → real DB error, abort and propagate
- `taken > 0` → name taken, return user-facing error
- `taken == 0` → name is free, proceed

The unused `existing dbEntity` variable is removed.

## Files

- **Modify:** `memory/memory.go` — replace lines 261–263 (the duplicate-name check inside `updateEntity`)
- **Modify:** `memory/memory_test.go` — add `TestUpdateEntity_NewNameAlreadyExists` test case

## Testing

One new test verifies the rejection path:

```go
func TestUpdateEntity_NewNameAlreadyExists(t *testing.T) {
    db, _ := OpenDB("")
    srv := NewMemoryService(db)

    // Create two entities.
    srv.CreateEntities([]Entity{
        {Name: "alpha", EntityType: "service"},
        {Name: "beta",  EntityType: "service"},
    })

    // Try to rename "alpha" to "beta" — must be rejected.
    err := srv.UpdateEntity("alpha", "beta", "")
    if err == nil {
        t.Fatal("expected error when renaming to an existing name, got nil")
    }

    // Both entities must still exist with original names.
    graph, _ := srv.ReadGraph()
    names := make(map[string]bool)
    for _, e := range graph.Entities {
        names[e.Name] = true
    }
    if !names["alpha"] || !names["beta"] {
        t.Errorf("expected both 'alpha' and 'beta' to still exist, got %v", names)
    }
}
```

No interface changes. No migration needed. Existing tests are unaffected.
