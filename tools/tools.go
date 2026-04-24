// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// withRecovery wraps an MCP tool handler to catch and log panics gracefully.

func withRecovery[A, R any](

	name string,

	handler func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error),

) func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args A) (res *mcp.CallToolResult, ret R, err error) {

		defer func() {

			if r := recover(); r != nil {

				slog.Error("[tools] MCP tool panicked", "tool", name, "panic", r, "stack", string(debug.Stack()))

				err = fmt.Errorf("internal server error in tool '%s' (panic recovered: %v)", name, r)

			}

		}()

		return handler(ctx, req, args)

	}

}

// withMetrics wraps a handler to record a call in ToolMetrics before execution.

func withMetrics[A, R any](

	name string,

	m *ToolMetrics,

	handler func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error),

) func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error) {

	return func(ctx context.Context, req *mcp.CallToolRequest, args A) (*mcp.CallToolResult, R, error) {

		m.Record(name)

		return handler(ctx, req, args)

	}

}

// Register adds all DocScout MCP tools to the server.

// graph, search, and semantic may be nil — tools that require them are omitted.

// metrics and docMetrics must not be nil.

// When readOnly is true, all graph mutation tools (create_entities, create_relations,

// add_observations, delete_*) are omitted; only read-only graph tools are registered.

func Register(s *mcp.Server, sc DocumentScanner, graph GraphStore, search ContentSearcher, semantic SemanticSearch, metrics *ToolMetrics, docMetrics *DocMetrics, readOnly bool) {

	// --- Scanner Tools ---

	mcp.AddTool(s, &mcp.Tool{

		Name: "list_repos",

		Description: "Lists repositories in the organization that contain documentation files. Accepts an optional file_type filter to narrow results to repos that contain a specific document type (e.g. 'openapi', 'asyncapi', 'proto', 'helm', 'dockerfile', 'readme'). Returns repo name, description, URL, file count, and the set of file types present.",
	}, withMetrics("list_repos", metrics, withRecovery("list_repos", listReposHandler(sc))))

	mcp.AddTool(s, &mcp.Tool{

		Name: "search_docs",

		Description: "Searches for documentation files by matching a query term against file paths and repository names. Accepts an optional file_type filter to narrow results to a specific document category (e.g. 'openapi', 'asyncapi', 'proto', 'readme', 'helm').",
	}, withMetrics("search_docs", metrics, withRecovery("search_docs", searchDocsHandler(sc))))

	mcp.AddTool(s, &mcp.Tool{

		Name: "get_file_content",

		Description: "Retrieves the raw content of a specific documentation file from a GitHub repository. Note: For security reasons, this tool will only return files that have been successfully indexed as documentation (i.e. returned by list_repos or search_docs).",
	}, withMetrics("get_file_content", metrics, withRecovery("get_file_content", getFileContentHandler(sc, docMetrics))))

	mcp.AddTool(s, &mcp.Tool{

		Name: "get_scan_status",

		Description: "Returns the current state of the documentation scanner and knowledge graph index. Call this before searching to confirm the index is populated, especially right after startup.",
	}, withMetrics("get_scan_status", metrics, withRecovery("get_scan_status", getScanStatusHandler(sc, graph, search, readOnly))))

	mcp.AddTool(s, &mcp.Tool{

		Name: "trigger_scan",

		Description: "Queues an immediate full repository scan without waiting for the next scheduled interval. " +

			"Use this after you know documentation has changed and you want to pick up the updates right away. " +

			"The scan runs asynchronously — call get_scan_status to monitor progress. " +

			"Duplicate triggers are coalesced: if a scan is already queued, this is a no-op (already_queued=true).",
	}, withMetrics("trigger_scan", metrics, withRecovery("trigger_scan", triggerScanHandler(sc))))

	if search != nil {

		mcp.AddTool(s, &mcp.Tool{

			Name: "search_content",

			Description: "Full-text search across the content of all cached documentation files. Use this to find which service handles a specific responsibility (e.g. 'payment', 'authentication'). Only available when SCAN_CONTENT=true.",
		}, withMetrics("search_content", metrics, withRecovery("search_content", searchContentHandler(search, docMetrics))))

	}

	// --- Memory / Knowledge Graph Tools ---

	if graph != nil {

		// Mutation tools are omitted when read-only mode is active.

		if !readOnly {

			mcp.AddTool(s, &mcp.Tool{

				Name: "create_entities",

				Description: "Create multiple new entities in the knowledge graph",
			}, withMetrics("create_entities", metrics, withRecovery("create_entities", createEntitiesHandler(graph, semantic))))

			mcp.AddTool(s, &mcp.Tool{

				Name: "create_relations",

				Description: "Create multiple new relations between entities",
			}, withMetrics("create_relations", metrics, withRecovery("create_relations", createRelationsHandler(graph))))

			mcp.AddTool(s, &mcp.Tool{

				Name: "add_observations",

				Description: "Add new observations to existing entities",
			}, withMetrics("add_observations", metrics, withRecovery("add_observations", addObservationsHandler(graph, semantic))))

			mcp.AddTool(s, &mcp.Tool{

				Name: "delete_entities",

				Description: fmt.Sprintf("Remove entities and their associated relations from the knowledge graph. Deleting more than %d entities in a single call requires confirm=true as a safety guard against accidental graph wipes.", massDeleteThreshold),
			}, withMetrics("delete_entities", metrics, withRecovery("delete_entities", deleteEntitiesHandler(graph, semantic))))

			mcp.AddTool(s, &mcp.Tool{

				Name: "delete_observations",

				Description: "Remove specific observations from entities",
			}, withMetrics("delete_observations", metrics, withRecovery("delete_observations", deleteObservationsHandler(graph))))

			mcp.AddTool(s, &mcp.Tool{

				Name: "delete_relations",

				Description: "Remove specific relations from the graph",
			}, withMetrics("delete_relations", metrics, withRecovery("delete_relations", deleteRelationsHandler(graph))))

		}

		mcp.AddTool(s, &mcp.Tool{

			Name: "update_entity",

			Description: "Rename an entity and/or change its type. When renaming, all relations and observations " +

				"that reference the entity are updated atomically — no data is lost. " +

				"Use this when a service is renamed (e.g. 'payment-service' → 'payments-svc') or reclassified (e.g. from 'service' to 'api').",
		}, withMetrics("update_entity", metrics, withRecovery("update_entity", updateEntityHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "read_graph",

			Description: "Read the entire knowledge graph",
		}, withMetrics("read_graph", metrics, withRecovery("read_graph", readGraphHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "list_entities",

			Description: "Returns all knowledge graph entities, optionally filtered by entity type. " +

				"Use this to enumerate all instances of a category: " +

				"entity_type='service' → all services, " +

				"entity_type='event-topic' → all Kafka/event topics, " +

				"entity_type='grpc-service' → all gRPC service contracts, " +

				"entity_type='team' → all teams, " +

				"entity_type='api' → all API entities. " +

				"Leave entity_type empty to list all entities (equivalent to read_graph entities only).",
		}, withMetrics("list_entities", metrics, withRecovery("list_entities", listEntitiesHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "list_relations",

			Description: "Returns relations from the knowledge graph, filtered by relation_type and/or from_entity. " +

				"Examples: " +

				"relation_type='depends_on' → all dependency edges; " +

				"relation_type='publishes_event' → all event producers; " +

				"relation_type='subscribes_event' → all event consumers; " +

				"from_entity='payment-svc' → all outgoing edges from payment-svc. " +

				"Both filters can be combined. Leave both empty to return all relations.",
		}, withMetrics("list_relations", metrics, withRecovery("list_relations", listRelationsHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "search_nodes",

			Description: "Search for nodes based on query",
		}, withMetrics("search_nodes", metrics, withRecovery("search_nodes", searchNodesHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "open_nodes",

			Description: "Retrieve specific nodes by name",
		}, withMetrics("open_nodes", metrics, withRecovery("open_nodes", openNodesHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "traverse_graph",

			Description: `Traverses the knowledge graph from a starting entity, following directed edges up to a given depth.















Use this instead of read_graph when you need to answer focused questions about a specific service — it returns only the relevant subgraph without loading every entity.















Examples:















  direction=outgoing, relation_type=depends_on  → transitive dependency tree of a service















  direction=incoming, relation_type=consumes_api → all services that consume a given API















  direction=both, depth=2                        → full two-hop neighbourhood of a service`,
		}, withMetrics("traverse_graph", metrics, withRecovery("traverse_graph", traverseGraphHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "get_integration_map",

			Description: "Returns the complete integration topology of a service in a single call: " +

				"which events it publishes and subscribes to, which APIs and gRPC services it exposes or depends on, " +

				"and which services it calls directly. Each entry includes a confidence level so the AI agent can " +

				"distinguish authoritative contract declarations (AsyncAPI, proto, OpenAPI) from inferred config values " +

				"(Spring Kafka, K8s env vars). Use this tool before any architecture, impact analysis, or documentation " +

				"task involving a specific service — it eliminates the need to read raw config files across multiple repos. " +

				"Check graph_coverage to know how much to trust the result.",
		}, withMetrics("get_integration_map", metrics, withRecovery("get_integration_map", getIntegrationMapHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "export_graph",

			Description: "Exports the entire knowledge graph as an interactive HTML visualization or JSON. " +

				"The HTML format produces a self-contained, offline-capable force-directed graph — " +

				"no internet connection required. Use output_path to write the file directly to disk " +

				"(e.g. output_path='/tmp/graph.html'), or omit it to receive the content inline. " +

				"Open the resulting HTML file in any browser to explore entities, relations, and observations.",
		}, withMetrics("export_graph", metrics, withRecovery("export_graph", exportGraphHandler(graph))))

		mcp.AddTool(s, &mcp.Tool{

			Name: "find_path",

			Description: `Finds the shortest connection path between two entities in the knowledge graph using undirected BFS.















Returns the ordered sequence of directed edges (from, relationType, to) that connect them, regardless of edge direction.















Use this to answer:















  - "How does service A connect to service B?"















  - "Is there any dependency chain between payment-svc and auth-svc?"















  - "What is the relationship path between team X and service Y?"















Returns found=false and an empty path when no connection exists within max_depth hops.















Complement to traverse_graph (which explores from one end) and get_integration_map (which shows a single service's topology).`,
		}, withMetrics("find_path", metrics, withRecovery("find_path", findPathHandler(graph))))

	}

	// --- Observability ---

	mcp.AddTool(s, &mcp.Tool{

		Name: "get_usage_stats",

		Description: "Returns how many times each MCP tool has been called and the top 20 most-fetched documents since server start. Use this to identify which documentation areas are most frequently accessed by AI agents, helping teams spot knowledge gaps.",
	}, withRecovery("get_usage_stats", getUsageStatsHandler(metrics, docMetrics)))

	// --- Semantic Search (Plus) ---

	if semantic != nil {

		mcp.AddTool(s, &mcp.Tool{

			Name: "semantic_search",

			Description: "Runs a natural-language semantic search over indexed documentation content and/or knowledge graph entities using vector embeddings. Returns results ranked by cosine similarity. Requires the server to be started with DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL set. Use 'target' to choose 'content', 'entities', or 'both'. Check stale_docs/stale_entities to know how many items are pending re-indexing.",
		}, withMetrics("semantic_search", metrics, withRecovery("semantic_search", semanticSearchHandler(semantic))))

	}

}
