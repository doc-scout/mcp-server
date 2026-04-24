# Skills

Project-level skills live in `.agent/skills/` and are loaded automatically by Claude Code.

## mcp-builder-go

**Trigger:** Any request to create, add, or extend an MCP tool/server in Go.

**Location:** `.agent/skills/mcp-builder-go/SKILL.md`

Covers:

- Server creation with `github.com/modelcontextprotocol/go-sdk`
- Typed handler pattern (args/result structs + closure factory)
- STDIO and HTTP transports
- Middleware (recovery, metrics)
- Dependency injection via closures
- Security (path traversal, input validation, bulk guards, auth)
- Concurrency (`sync.WaitGroup`, semaphore, `sync.RWMutex`)
