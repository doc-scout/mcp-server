# DocScout-MCP Development Guidelines

Welcome to the DocScout-MCP project! When contributing to this Model Context Protocol (MCP) server, please strictly adhere to the following guidelines, which are aligned with the official [Model Context Protocol standards](https://modelcontextprotocol.io/docs/develop/build-server) and the [Official Go SDK](https://github.com/modelcontextprotocol/go-sdk).

## 1. Official Go SDK Usage

**All architectural changes and refactors must target the official SDK**:
`github.com/modelcontextprotocol/go-sdk`

Whenever introducing new features (Prompts, Resources, or new Tools), consult the official Go SDK documentation to ensure spec compliance.

## 2. Standard I/O (stdio) Constraints

Because this server communicates with AI clients (Claude Desktop, VS Code, Antigravity) via the Standard Input/Output (`stdio`) transport:
- **NEVER write application logs, debug messages, or arbitrary text to `stdout` (`fmt.Println`, `fmt.Printf`, `os.Stdout`).**
- Writing to `stdout` will corrupt the JSON-RPC stream, causing the AI client to fail to parse the messages and crash the connection.
- **ALWAYS use `log.Printf` or `fmt.Fprintln(os.Stderr, ...)`**. Go's standard `log` package writes to `stderr` by default, which is completely safe and properly separated from the JSON-RPC communication line.

## 3. Security and Sandboxing (Critical)

MCP servers execute code locally on behalf of an LLM. Treat the LLM as an untrusted user.
- **Path Traversal:** Always validate requested file paths. A tool should never allow an LLM to read outside of its designated scope (e.g., trying to read `../../../../etc/passwd` or `~/.ssh/id_rsa`).
- Emulate our internal `IsIndexed()` security model: Do not fetch arbitrary files from GitHub just because the LLM asked for it; only fetch files that were explicitly whitelisted and indexed during the namespace scan.

## 4. Error Handling

- Return meaningful, human-readable errors back to the LLM so it knows how to self-correct its next tool call.
- For internal unrecoverable errors during a tool call, log the stack trace to `stderr` and return a safe, generic error string to the MCP response context.

## 5. Tool Design

- **Descriptions:** Write highly descriptive tool descriptions. The LLM relies 100% on the description you map in the Tool Definition to understand *when* and *how* to use it.
- **Arguments:** Keep JSON Schema arguments strict. Include `description` fields on every single argument property to guide the LLM.

## 6. Testing

- Keep unit tests focused on the underlying logic (e.g., GitHub Scanner, Cache updates) rather than the JSON-RPC transport wrapper.
- Use explicit API mocks for GitHub.
- Every new parser in `internal/infra/github/parser/` must have a corresponding `*_test.go` covering at least: valid input, missing required fields, edge cases (empty deps, all-excluded scopes).

## 7. Go Version

The project targets **Go 1.26+** (declared in `go.mod`). Use modern language features where they improve clarity. Run `govulncheck ./...` to check for known vulnerabilities before submitting.

## 8. Adding New Parsers

Follow the established pattern when adding support for a new manifest format:

1. **Parser** — create `internal/infra/github/parser/<format>.go` implementing the `FileParser` interface (`FileType()`, `Filenames()`, `Parse()`).
2. **Tests** — create `internal/infra/github/parser/<format>_test.go` with table-driven tests.
3. **Register** — call `parser.Register(New<Format>Parser())` in `internal/app/wire.go`; the `AutoIndexer` picks it up automatically.
4. **Docs** — update `docs/how-it-works.md` and `README.md`.
5. **Docs** — update `docs/how-it-works.md` and `README.md`.

## 9. Graph Safety Rules

- **Observation filtering**: All user-originated observations pass through `sanitizeObservations` (in the graph adapter layer) before reaching the store. This rejects empty, too-short (< 2 chars), too-long (> 500 chars), and within-batch duplicate observations.
- **Audit log**: `GraphAuditLogger` (in `internal/app/`) wraps the store in `internal/app/wire.go` and emits a structured slog line for every mutation. Do not bypass this wrapper.
- **Mass-delete guard**: `delete_entities` rejects batches of more than 10 entities unless `confirm: true` is set. Apply the same pattern to any new bulk-destructive tool.

## 10. Deployment Targets

Deployment assets live under `deploy/`:

| Directory | Target |
|-----------|--------|
| `deploy/k8s/` | Raw Kubernetes manifests (apply with `kubectl` or `make k8s-deploy`) |
| `deploy/helm/` | Helm chart v2 (install with `helm install` or `make helm-install`) |
| `deploy/terraform/` | Kubernetes Terraform module (`hashicorp/kubernetes` provider) |

The `Makefile` at the project root provides targets for all deployment paths. The `docker-compose.yml` supports three profiles: `http` (SQLite), `postgres`, and `stdio`.
