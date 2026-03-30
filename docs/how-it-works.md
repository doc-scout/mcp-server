# How DocScout-MCP Works

DocScout-MCP is designed to bridge the gap between large, distributed software systems and AI agents. It does this by creating a reliable, deterministic model of your architecture and exposing it—alongside raw documentation content—to any LLM via the Model Context Protocol (MCP).

This document explains the two core mechanisms that make this possible: the **Deterministic Dependency Graph** and the **MCP Interfaces**.

---

## 1. The Deterministic Dependency Graph

A common problem with feeding codebases to AI is "hallucination"—the AI guesses how services are connected based on naming conventions or outdated `README.md` files. DocScout-MCP eliminates this by building a **Deterministic Dependency Graph**.

Rather than relying on the AI to infer relationships, the project collects declarative architectural metadata during its repository scanning phase.

### How `AutoIndexer` Works

1. **Discovery & Parsing**: As the scanner sweeps through attached repositories, it looks for Backstage-compatible `catalog-info.yaml` files. The `scanner/parser` package extracts structured metadata from these files.
2. **Graph Upsert**: The `AutoIndexer` takes these parsed components (such as a system, component, or API) and maps them into `Entities`, `Relations`, and `Observations`. These are then persisted into the core SQLite Knowledge Graph via `GraphWriter`.
3. **Soft-Delete Lifecycle**: Because this process is completely deterministic, auto-generated entities receive special tracking annotations (e.g., `_source:catalog-info` and `_scan_repo:<repo_name>`). If a repository is removed from the scan in the future, the indexer adds a `_status:archived` flag to its entities rather than hard-deleting them, preserving historical context.

### Example: A Service Definition

If a repository contains the following `catalog-info.yaml`:

```yaml
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: payment-service
  description: Handles payment
spec:
  type: service
  lifecycle: production
  owner: team-payments
  dependsOn:
    - component:db
```

The indexer deterministic translates this to:
- **Entity**: `payment-service` (Type: `service`).
- **Observations**: `_source:catalog-info`, `_scan_repo:org/payment-service`, `description: Handles payment`, `lifecycle: production`, `owner: team-payments`.
- **Relation**: A `depends_on` edge pointing from `payment-service` to `component:db`.

Now, the architecture is a fact stored in the database, not an assumption.

---

## 2. Exposing Superpowers via MCP

If the deterministic graph is the "brain," then the **Model Context Protocol (MCP)** is the "API" that agents use to interact with it.

DocScout-MCP registers several tools over `stdio` or HTTP transport, allowing agents (such as Claude Desktop or custom tools) to query the exact state of the universe.

### Key Agent Tools

The server registered tools are defined in the `tools` package and include:
- `search_nodes`: Allows the agent to search for specific entities or relations in the graph.
- `open_nodes`: Provide the agent deep-dive observational data about a given entity.
- `read_graph`: Dumps the underlying graph or subgraph.
- `search_content` / `get_file_content`: Fetches the raw content of cached files (like `README.md` or API specs) associated with the discovered services.

### Practical Example: Answering Architectural Queries

Imagine you ask an AI Agent a question:
> *"What happens if I shut down the `component:db`? Which systems will go offline, and who should I notify?"*

Instead of reading all source code simultaneously, the Agent performs a surgical sequence of MCP tool calls:

1. **Invoke `search_nodes`**: The agent queries `search_nodes` with `"component:db"`.
2. **Examine Relations**: DocScout-MCP searches the internal SQLite database and responds with a JSON-RPC payload displaying an explicit `depends_on` relation from `payment-service` to `component:db`.
3. **Expand Context via `open_nodes`**: The agent expands the `payment-service` node and sees the `owner: team-payments` observation.
4. **Formulate Accurate Answer**: The agent responds confidently:
   *"If you disable `component:db`, the `payment-service` will go offline because it explicitly depends on it. You should notify `team-payments` before taking any action. Would you like me to fetch their onboarding docs using DocScout's documentation tools?"*

### Summary
By pairing a **Deterministic Dependency Graph** mapped from declarative manifests with standard **MCP Tools**, DocScout-MCP provides agents with perfect, non-alucinated context of your entire distributed environment.
