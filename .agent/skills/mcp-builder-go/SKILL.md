---
name: mcp-builder-go
description: Use when building MCP servers in Go using github.com/modelcontextprotocol/go-sdk. Covers tool definition, handler patterns, STDIO/HTTP transports, middleware, and security. Triggers on any request to create, add, or extend an MCP server written in Go.
---

# Building MCP Servers in Go

## Overview

Use `github.com/modelcontextprotocol/go-sdk` to build efficient, idiomatic MCP servers in Go.
The critical rule: **NEVER write free text to `os.Stdout`** — the MCP protocol uses stdout for JSON-RPC. Any stray output corrupts the stream.

## Core Setup

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"

func main() {
    // All logging MUST go to stderr
    log.SetOutput(os.Stderr)
    slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

    server := mcp.NewServer(&mcp.Implementation{
        Name:    "my-mcp-server",
        Version: "1.0.0",
    }, nil)

    // Register tools
    registerTools(server)

    // Start STDIO transport (blocks until stdin closes)
    ctx := context.Background()
    if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
        log.Fatal(err)
    }
}
```

## Tool Definition Pattern

### Typed Args + Result Structs

```go
type GetDocArgs struct {
    Path string `json:"path" jsonschema:"description=Relative path to the document,example=docs/api.md"`
}

type GetDocResult struct {
    Content  string `json:"content"`
    FilePath string `json:"file_path"`
}

func getDocHandler(store DocStore) func(
    ctx context.Context,
    req *mcp.CallToolRequest,
    args GetDocArgs,
) (*mcp.CallToolResult, GetDocResult, error) {
    return func(ctx context.Context, req *mcp.CallToolRequest, args GetDocArgs) (*mcp.CallToolResult, GetDocResult, error) {
        // Validate at the boundary — never trust LLM input
        if strings.TrimSpace(args.Path) == "" {
            return nil, GetDocResult{}, fmt.Errorf("'path' must not be empty")
        }

        content, err := store.Get(args.Path)
        if err != nil {
            return nil, GetDocResult{}, fmt.Errorf("reading %q: %w", args.Path, err)
        }

        return nil, GetDocResult{Content: content, FilePath: args.Path}, nil
    }
}
```

### Register the Tool

```go
mcp.AddTool(server, &mcp.Tool{
    Name:        "get_doc",
    Description: "Retrieves the content of a documentation file by path. Use this to read a specific file after finding it with search_docs.",
}, getDocHandler(store))
```

**Tool description guidelines:**
- Explain WHAT the tool does and WHEN the LLM should call it
- Include relationships to other tools ("after finding it with search_docs")
- Be precise — the LLM relies on this to select the right tool

## Handler Signature

```
func(ctx, req, args ArgsType) (*mcp.CallToolResult, ResultType, error)
```

- First return is `*mcp.CallToolResult` — return `nil` for standard JSON results; use it only when you need raw text/blob responses
- Second return is your typed result struct — serialized to JSON automatically
- `error` propagates as a tool error response the LLM can read

## Middleware

Wrap every tool with recovery and observability:

```go
// Recovery middleware — catches panics, logs stack trace to stderr
func withRecovery(name string, h mcp.ToolHandlerFunc) mcp.ToolHandlerFunc {
    return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        defer func() {
            if r := recover(); r != nil {
                slog.Error("tool panic", "tool", name, "panic", r,
                    "stack", string(debug.Stack()))
            }
        }()
        return h(ctx, req)
    }
}

// Metrics middleware — count calls before execution
func withMetrics(name string, counter *atomic.Int64, h mcp.ToolHandlerFunc) mcp.ToolHandlerFunc {
    return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        counter.Add(1)
        return h(ctx, req)
    }
}

// Compose when registering
mcp.AddTool(server, &mcp.Tool{Name: "get_doc", Description: "..."}, 
    withMetrics("get_doc", counter, withRecovery("get_doc", getDocHandler(store))))
```

## HTTP Transport (Optional)

```go
handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
    return server
}, nil)

mux := http.NewServeMux()
mux.Handle("/mcp", authMiddleware(handler))
mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
    w.WriteHeader(http.StatusOK)
})

srv := &http.Server{Addr: ":8080", Handler: mux}
go srv.ListenAndServe()

// Graceful shutdown
<-ctx.Done()
shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
srv.Shutdown(shutdownCtx)
```

**Bearer token auth (use constant-time compare to prevent timing attacks):**
```go
func authMiddleware(next http.Handler) http.Handler {
    token := os.Getenv("MCP_TOKEN")
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
        if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Dependency Injection

Use closures — never use global state:

```go
type Services struct {
    Docs  DocStore
    Graph GraphStore   // may be nil
    Cache ContentCache // may be nil
}

func registerTools(s *mcp.Server, svc Services) {
    mcp.AddTool(s, &mcp.Tool{Name: "get_doc", Description: "..."}, getDocHandler(svc.Docs))

    // Conditional registration — only register if dependency exists
    if svc.Graph != nil {
        mcp.AddTool(s, &mcp.Tool{Name: "create_entities", Description: "..."}, createEntitiesHandler(svc.Graph))
    }
}
```

## Security Checklist

- **Path traversal:** Never pass user-provided paths directly to `os.ReadFile`. Validate against an allowed set (e.g., cached file list) or use `filepath.Rel` to confirm the path stays within the designated root.
- **Input sanitization:** Strip/reject empty, too-short, or structurally invalid inputs before reaching the store.
- **Bulk operation guards:** For tools that delete or mutate many items, enforce a `maxItems` threshold and return an error if exceeded.
- **Auth:** Use constant-time comparison for tokens (`crypto/subtle`). Validate webhook signatures with HMAC-SHA256.
- **Stdout purity:** Double-check all transitive dependencies — some libraries print to stdout on init. Redirect with `os.Stdout = os.Stderr` as a last resort.

## Concurrency Patterns

```go
// Fan-out with bounded concurrency
sem := make(chan struct{}, 10) // max 10 goroutines
var wg sync.WaitGroup
var mu sync.Mutex
var results []Result

for _, item := range items {
    wg.Add(1)
    go func(it Item) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        r, err := process(it)
        if err != nil {
            slog.Error("process failed", "item", it, "err", err)
            return
        }
        mu.Lock()
        results = append(results, r)
        mu.Unlock()
    }(item)
}
wg.Wait()

// Protect shared caches
type Cache struct {
    mu sync.RWMutex
    m  map[string]Entry
}
func (c *Cache) Get(k string) (Entry, bool) {
    c.mu.RLock(); defer c.mu.RUnlock()
    e, ok := c.m[k]; return e, ok
}
func (c *Cache) Set(k string, v Entry) {
    c.mu.Lock(); defer c.mu.Unlock()
    c.m[k] = v
}
```

## Quick Reference

| Task | API |
|------|-----|
| Create server | `mcp.NewServer(&mcp.Implementation{Name, Version}, nil)` |
| Register tool | `mcp.AddTool(server, &mcp.Tool{Name, Description}, handler)` |
| Start STDIO | `server.Run(ctx, &mcp.StdioTransport{})` |
| Start HTTP | `mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)` |
| Log safely | `slog.Info(...)` / `log.Println(...)` — both write to stderr by default |
| Fatal log | `log.Fatal(...)` — writes to stderr then exits |

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| `fmt.Println(...)` in any path | Use `slog.Info(...)` or `fmt.Fprintln(os.Stderr, ...)` |
| Global vars for dependencies | Inject via closure factory functions |
| Registering tool when dep is nil | Guard with `if dep != nil { mcp.AddTool(...) }` |
| Passing LLM path directly to `os.ReadFile` | Validate against allowlist first |
| Single goroutine for bulk ops | Use `sync.WaitGroup` + semaphore |
| Plain `ConstantTimeCompare` on empty token | Check token is non-empty at startup; fail fast |
