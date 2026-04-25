// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package audit_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coregraph "github.com/doc-scout/mcp-server/internal/core/graph"
	infradb "github.com/doc-scout/mcp-server/internal/infra/db"
	"github.com/doc-scout/mcp-server/tests/testutils"
	adaptermcp "github.com/doc-scout/mcp-server/internal/adapter/mcp"
)

// setupAuditServer creates a test MCP server with a live AuditStore.

func setupAuditServer(t *testing.T) (*mcp.ClientSession, memory.AuditStore) {

	t.Helper()

	ctx := t.Context()

	db, err := infradb.OpenDB("")

	if err != nil {

		t.Fatalf("OpenDB: %v", err)

	}

	auditStore, err := memory.NewAuditStore(db)

	if err != nil {

		t.Fatalf("NewAuditStore: %v", err)

	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)

	memorySrv := coregraph.NewMemoryService(infradb.NewGraphRepo(db))

	agentFn := func() string { return "test-agent" }

	auditedGraph := tools.NewGraphAuditLogger(memorySrv, agentFn, auditStore)

	adaptermcp.Register(server, &testutils.MockScanner{}, auditedGraph, nil, nil,

		adaptermcp.NewToolMetrics(), adaptermcp.NewDocMetrics(), nil, false, auditStore)

	t1, t2 := mcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, t1, nil); err != nil {

		t.Fatalf("server.Connect: %v", err)

	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)

	session, err := client.Connect(ctx, t2, nil)

	if err != nil {

		t.Fatalf("client.Connect: %v", err)

	}

	return session, auditStore

}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {

	t.Helper()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})

	if err != nil {

		t.Fatalf("%s: %v", name, err)

	}

	if res.IsError {

		t.Fatalf("%s returned MCP error: %v", name, res.Content)

	}

	if len(res.Content) == 0 {

		return "{}"

	}

	text, ok := res.Content[0].(*mcp.TextContent)

	if !ok {

		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])

	}

	return text.Text

}

func TestAudit_CreateEntitiesAppearsInQueryLog(t *testing.T) {

	session, store := setupAuditServer(t)

	callTool(t, session, "create_entities", map[string]any{

		"entities": []map[string]any{

			{"name": "svc-audit", "entityType": "service", "observations": []string{"audit test"}},
		},
	})

	time.Sleep(10 * time.Millisecond)

	events, total, err := store.Query(t.Context(), memory.AuditFilter{Tool: "create_entities"})

	if err != nil {

		t.Fatalf("Query: %v", err)

	}

	if total == 0 || len(events) == 0 {

		t.Fatal("expected at least one audit event for create_entities")

	}

	if events[0].Agent != "test-agent" {

		t.Fatalf("want agent=test-agent, got %q", events[0].Agent)

	}

	if events[0].Outcome != "ok" {

		t.Fatalf("want outcome=ok, got %q", events[0].Outcome)

	}

}

func TestAudit_QueryAuditLogMCPTool(t *testing.T) {

	session, _ := setupAuditServer(t)

	callTool(t, session, "create_entities", map[string]any{

		"entities": []map[string]any{

			{"name": "svc-x", "entityType": "service", "observations": []string{"x"}},
		},
	})

	raw := callTool(t, session, "query_audit_log", map[string]any{

		"tool": "create_entities",
	})

	var result tools.QueryAuditLogResult

	if err := json.Unmarshal([]byte(raw), &result); err != nil {

		t.Fatalf("unmarshal QueryAuditLogResult: %v — raw: %s", err, raw)

	}

	if result.Total == 0 {

		t.Fatal("expected total > 0")

	}

}

func TestAudit_GetAuditSummaryRiskyMassDelete(t *testing.T) {

	session, store := setupAuditServer(t)

	_ = store.Write(t.Context(), memory.AuditEvent{

		Agent: "bot", Tool: "delete_entities", Operation: "delete",

		Targets: memory.MarshalTargets([]string{}), Count: 15, Outcome: "ok",
	})

	raw := callTool(t, session, "get_audit_summary", map[string]any{})

	var result tools.GetAuditSummaryResult

	if err := json.Unmarshal([]byte(raw), &result); err != nil {

		t.Fatalf("unmarshal GetAuditSummaryResult: %v — raw: %s", err, raw)

	}

	if result.Summary.TotalMutations == 0 {

		t.Fatal("expected total_mutations > 0")

	}

	if len(result.Summary.RiskyEvents) == 0 {

		t.Fatal("expected mass delete to appear in risky_events")

	}

}

func TestAudit_OutcomeError(t *testing.T) {

	session, store := setupAuditServer(t)

	_ = session

	_ = store.Write(t.Context(), memory.AuditEvent{

		Agent: "test-agent", Tool: "delete_entities", Operation: "delete",

		Targets: memory.MarshalTargets([]string{"missing"}), Count: 1,

		Outcome: "error", ErrorMsg: "entity not found",
	})

	events, total, err := store.Query(t.Context(), memory.AuditFilter{Outcome: "error"})

	if err != nil {

		t.Fatalf("Query: %v", err)

	}

	if total == 0 {

		t.Fatal("expected error outcome event")

	}

	if events[0].ErrorMsg != "entity not found" {

		t.Fatalf("want ErrorMsg='entity not found', got %q", events[0].ErrorMsg)

	}

}
