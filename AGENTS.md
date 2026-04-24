You are an expert Go Developer and a specialist in the Model Context Protocol (MCP).
When contributing to this project (DocScout-MCP), follow these strict architectural and coding guidelines:

# 1. MCP Standard Library
We are standardizing around the official Google/Anthropic Go SDK for MCP.
When suggesting code, architectural changes, or adding new Tools/Resources/Prompts, you MUST refer to the interface design of `github.com/modelcontextprotocol/go-sdk`.

# 2. STDIO Transport Strict Safety (CRITICAL)
This MCP server runs via Standard Input/Output (`stdio`). The JSON-RPC messages are passed via `stdout` and `stdin`.
- **CRITICAL:** NEVER use `fmt.Println`, `fmt.Printf`, or any other function that writes free text to `os.Stdout`.
- Any non-JSON-RPC text written to `stdout` corrupts the communication stream and breaks the connection with the client (e.g., Claude Desktop, VS Code, Google Antigravity).
- **Log specifically to STDERR:** Use Go's standard `log` package (e.g., `log.Println`, `log.Printf`), which automatically writes to `os.Stderr`. If using `fmt`, do `fmt.Fprintln(os.Stderr, "message")`.

# 3. Security and Path Traversal
- Assume all inputs from the LLM client are unverified.
- Never blindly pass paths to `os.ReadFile` or `client.Repositories.GetContents`. Always validate against the internal namespace/cache to prevent path traversal (e.g., reading sensitive files outside of the `docs/` designation).
- For internal API calls failing, return a descriptive error that the LLM can read in the MCP Tool Response, but log the verbose stack trace to `stderr`.

# 4. Tool Design
- The LLM relies heavily on descriptions. Always add precise, detailed `Description` strings to every `Tool` definition.
- For tool JSON schema inputs, always provide a property-level `description` field for every argument.

# 5. Idiomatic Go
- Treat the project like a production backend system: Use Goroutines correctly (sync.WaitGroup, semaphores to cap concurrency), leverage `sync.RWMutex` for caches, and fail gracefully.
- Support Go 1.26+. The project's `go.mod` declares `go 1.26.1`.

# 6. Knowledge Graph Integrity
- All mutations to the graph (via `create_entities`, `add_observations`) pass through `sanitizeObservations` before reaching the store. Never bypass this layer.
- The `GraphAuditLogger` decorator wraps the store in `main.go` and logs every mutation to slog. Preserve this wiring when adding new graph operations.
- The `delete_entities` tool enforces a mass-delete guard (`massDeleteThreshold = 10`). Any new bulk-delete tools must implement the same pattern.

# 7. Scanner Extension Points

New manifest parsers implement the `FileParser` interface in `scanner/parser/extension.go` and register with the global `parser.Default` registry via `parser.Register()` in `main.go`.

## FileParser Interface

```go
// FileParser is the extension point for manifest parsers.
type FileParser interface {
    FileType() string        // unique classifier key (e.g. "pipfile")
    Filenames() []string     // root-level filenames (e.g. ["Pipfile"]) or suffix sentinels (e.g. [".proto"])
    Parse(data []byte) (ParsedFile, error)
}
```

## ParsedFile

```go
type ParsedFile struct {
    EntityName   string           // primary entity name; empty if only AuxEntities
    EntityType   string           // defaults to "service" if blank
    Observations []string         // parser-specific observations (e.g. "version:1.2.3")
    Relations    []ParsedRelation // directed edges; To="" → filled with repo service name
    MergeMode    MergeMode        // MergeModeUpsert (default) or MergeModeCatalog
    AuxEntities  []AuxEntity      // additional entities (used by codeowners-style parsers)
}
```

## Registration Pattern

```go
// In main.go — register at startup before scanner/indexer construction:
parser.Register(parser.GoModParser())
parser.Register(myorg.NewPipfileParser())

// Custom parser in mypkg/pipfile/parser.go:
type Parser struct{}
func (p *Parser) FileType()  string   { return "pipfile" }
func (p *Parser) Filenames() []string { return []string{"Pipfile"} }
func (p *Parser) Parse(data []byte) (parser.ParsedFile, error) { ... }
```

## Conventions

- `Register()` panics on duplicate `FileType()` or filename — caught at startup, not runtime.
- `Parse()` returning an error causes the file to be **skipped** with a warning log (scan continues).
- Auto-observations `_source:<FileType>` and `_scan_repo:<repo.FullName>` are added by the indexer; parsers must not duplicate them.
- For suffix-based discovery (e.g. `*.proto`), include a sentinel like `".proto"` in `Filenames()` — the scanner matches files whose name ends with that suffix.
- Infra directories (`deploy/`, `.github/workflows/`) are scanned by `scanInfraDir`; root-level filenames are scanned by `scanRepo`. New parsers targeting root-level files only need to add their filenames via `Filenames()`.

## Integration Relation Types

| Relation | From | To | Source |
|---|---|---|---|
| `publishes_event` | service | event-topic | AsyncAPI |
| `subscribes_event` | service | event-topic | AsyncAPI |
| `exposes_api` | service | api | OpenAPI/Swagger |
| `provides_grpc` | service | grpc-service | .proto |
| `depends_on_grpc` | service | grpc-service | .proto imports |
| `calls_service` | service | service | K8s env vars |
| `uses_mcp` | service | mcp-server | mcp-config parser |

New entity types: `event-topic`, `grpc-service`, `mcp-server` (in addition to existing `api`, `service`, `team`, `person`).

## `get_integration_map` tool

Use `get_integration_map` to answer architecture, impact, and documentation questions about a specific service's integration topology. It returns all integration edges in a single call including a `graph_coverage` field:

- `"full"` — at least one authoritative source (AsyncAPI, proto, OpenAPI) and no inferred sources
- `"partial"` — mix of authoritative and inferred sources
- `"inferred"` — all relations come from config heuristics (Spring Kafka, K8s env vars)
- `"none"` — no integration data found for this service
