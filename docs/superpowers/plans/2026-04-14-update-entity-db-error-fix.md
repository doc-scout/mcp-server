# updateEntity DB Error Swallowing Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix a bug where a real DB error during the `updateEntity` duplicate-name check is silently swallowed, allowing a rename to proceed incorrectly.

**Architecture:** Replace the `gorm.First`-based duplicate-name check (which conflates "not found" with real errors) with a `COUNT(*)`-based check — the same pattern used throughout the rest of `memory.go`. Real DB errors are now propagated; `taken > 0` means the name is taken; `taken == 0` means it's free.

**Tech Stack:** Go 1.26, GORM, SQLite (tests), PostgreSQL (production-compatible).

---

### Task 1: Write the failing test

**Files:**
- Modify: `memory/memory_test.go` (append after last test)

- [ ] **Step 1: Add the test**

Append this function to `memory/memory_test.go`:

```go
func TestUpdateEntity_NewNameAlreadyExists(t *testing.T) {
	srv := newTestService(t)

	// Create two entities.
	_, err := srv.CreateEntities([]Entity{
		{Name: "alpha", EntityType: "service"},
		{Name: "beta", EntityType: "service"},
	})
	if err != nil {
		t.Fatalf("CreateEntities failed: %v", err)
	}

	// Renaming "alpha" to "beta" must be rejected — "beta" already exists.
	err = srv.UpdateEntity("alpha", "beta", "")
	if err == nil {
		t.Fatal("expected error when renaming to an existing entity name, got nil")
	}

	// Both entities must still exist with their original names.
	graph, err := srv.ReadGraph()
	if err != nil {
		t.Fatalf("ReadGraph failed: %v", err)
	}
	names := make(map[string]bool, len(graph.Entities))
	for _, e := range graph.Entities {
		names[e.Name] = true
	}
	if !names["alpha"] {
		t.Error("expected entity 'alpha' to still exist after rejected rename")
	}
	if !names["beta"] {
		t.Error("expected entity 'beta' to still exist after rejected rename")
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./memory/... -run TestUpdateEntity_NewNameAlreadyExists -v
```

Expected output:
```
--- FAIL: TestUpdateEntity_NewNameAlreadyExists
    memory_test.go:NNN: expected error when renaming to an existing entity name, got nil
FAIL
```

The test fails because the current code swallows the DB "not found" result and never returns an error when the target name exists.

---

### Task 2: Fix the duplicate-name check

**Files:**
- Modify: `memory/memory.go:258–263`

- [ ] **Step 3: Replace the `First`-based check with `COUNT(*)`**

In `memory/memory.go`, inside `updateEntity`, replace lines 260–263:

**Remove:**
```go
			// Ensure the new name is not already taken.
			var existing dbEntity
			if err := tx.Where("name = ?", newName).First(&existing).Error; err == nil {
				return fmt.Errorf("entity %q already exists", newName)
			}
```

**Replace with:**
```go
			// Ensure the new name is not already taken.
			var taken int64
			if err := tx.Model(&dbEntity{}).Where("name = ?", newName).Count(&taken).Error; err != nil {
				return fmt.Errorf("checking entity name %q: %w", newName, err)
			}
			if taken > 0 {
				return fmt.Errorf("entity %q already exists", newName)
			}
```

The full `updateEntity` function after the edit (for reference):

```go
func (s store) updateEntity(oldName, newName, newType string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Verify the entity exists.
		var entity dbEntity
		if err := tx.Where("name = ?", oldName).First(&entity).Error; err != nil {
			return fmt.Errorf("entity %q not found: %w", oldName, err)
		}

		if newName != "" && newName != oldName {
			// Ensure the new name is not already taken.
			var taken int64
			if err := tx.Model(&dbEntity{}).Where("name = ?", newName).Count(&taken).Error; err != nil {
				return fmt.Errorf("checking entity name %q: %w", newName, err)
			}
			if taken > 0 {
				return fmt.Errorf("entity %q already exists", newName)
			}

			// Update relations.
			if err := tx.Model(&dbRelation{}).Where("from_node = ?", oldName).Update("from_node", newName).Error; err != nil {
				return err
			}
			if err := tx.Model(&dbRelation{}).Where("to_node = ?", oldName).Update("to_node", newName).Error; err != nil {
				return err
			}
			// Update observations.
			if err := tx.Model(&dbObservation{}).Where("entity_name = ?", oldName).Update("entity_name", newName).Error; err != nil {
				return err
			}
			// Rename the entity (change primary key).
			if err := tx.Model(&dbEntity{}).Where("name = ?", oldName).Update("name", newName).Error; err != nil {
				return err
			}
			// For subsequent type update, work on the new name.
			oldName = newName
		}

		if newType != "" && newType != entity.EntityType {
			if err := tx.Model(&dbEntity{}).Where("name = ?", oldName).Update("entity_type", newType).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
```

- [ ] **Step 4: Run the new test — must pass now**

```bash
go test ./memory/... -run TestUpdateEntity_NewNameAlreadyExists -v
```

Expected output:
```
--- PASS: TestUpdateEntity_NewNameAlreadyExists (0.00s)
PASS
```

- [ ] **Step 5: Run the full test suite — no regressions**

```bash
go test ./... 2>&1 | grep -E "FAIL|ok"
```

Expected: all lines start with `ok`, none with `FAIL`.

- [ ] **Step 6: Commit**

```bash
git add memory/memory.go memory/memory_test.go
git commit -m "fix(memory): replace First with COUNT in updateEntity duplicate-name check

gorm.First returns ErrRecordNotFound on miss, causing any other DB error
to be silently swallowed and the rename to proceed incorrectly.
COUNT(*) has unambiguous semantics: err != nil is always a real failure,
taken > 0 means the name is taken. Matches the pattern used throughout
the rest of the file."
```
