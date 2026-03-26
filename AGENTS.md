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
- Support Go 1.22+.
