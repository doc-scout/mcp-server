# Integration Topology Discovery — Design Spec

**Date:** 2026-04-03
**Status:** Approved
**Goal:** Enable AI agents to answer distributed-system integration questions ("who consumes this event?", "what services does checkout-service call?") in a single MCP tool call, by automatically populating the knowledge graph with producer/consumer and API dependency relationships during each scan.

---

## Problem

The knowledge graph today has `depends_on` edges from package manifests (go.mod, pom.xml), but no understanding of *runtime integrations*: who publishes to Kafka topic `order.created`, who subscribes, which service exposes a gRPC endpoint, which calls an HTTP API. Without these edges, an AI agent must read multiple raw files across dozens of repos to reconstruct the integration topology — burning tokens and introducing latency with no guarantee of completeness.

The design principle is: **one tool call = complete, actionable answer**. DocScout pre-computes the topology so the AI never has to.

---

## Solution: Multi-Protocol Integration Scanner + `get_integration_map` Tool

Two complementary layers:

- **Layer 1 — Parsers:** Five new `FileParser` implementations (from the #13 extension point) that extract producer/consumer/dependency relationships from canonical protocol files and config.
- **Layer 2 — Tool:** A new `get_integration_map` MCP tool that aggregates all integration edges for a given service into a single structured response, including a `graph_coverage` field so the AI knows how much to trust the answer.

---

## Architecture

```
GitHub Repos
    │
    ├─ asyncapi.yaml/json   ─→ AsyncAPIParser      ─┐
    ├─ application.yml/     ─→ SpringKafkaParser    ─┤
    │  application.props                             │
    ├─ openapi.yaml/json,   ─→ OpenAPIParser        ─┼─→ AutoIndexer ─→ Knowledge Graph
    │  swagger.json                                  │
    ├─ *.proto              ─→ ProtoParser           ─┤
    └─ deploy/**/*.yaml     ─→ K8sServiceParser     ─┘
                                                          │
                                       ┌──────────────────┘
                                       ▼
                              New graph entities:
                              - event-topic  (e.g. "order.created")
                              - grpc-service (e.g. "PaymentService")

                              New graph relations:
                              - publishes_event   service → event-topic
                              - subscribes_event  service → event-topic
                              - provides_grpc     service → grpc-service
                              - depends_on_grpc   service → grpc-service
                              - exposes_api       service → api (enriched)
                              - calls_service     service → service

                                       │
                                       ▼
                              get_integration_map tool
                              (one call, full picture)
```

**Dependencies:**
- Requires `#13 Custom Parser Extension` — all five parsers implement `FileParser`.
- `#14 traverse_graph` is complementary but **not a prerequisite** — `get_integration_map` runs its own SQL-level aggregation via `memory/integration.go`.

---

## Layer 1: Parsers

All parsers implement `FileParser` from `scanner/parser/extension.go`.

### Source Confidence Model

Each relation produced by a parser carries a source confidence observation on the originating entity:

| Parser | Confidence | Rationale |
|---|---|---|
| `AsyncAPIParser` | `authoritative` | Explicit contract declaration |
| `ProtoParser` | `authoritative` | Explicit service contract |
| `OpenAPIParser` | `authoritative` | Explicit API contract |
| `SpringKafkaParser` | `inferred` | Config-derived, naming may vary |
| `K8sServiceParser` | `inferred` | Env var heuristic |

Stored as observation `_integration_source:asyncapi` (or `spring-kafka`, `proto`, `openapi`, `k8s-env`) on the service entity. The `get_integration_map` tool reads these to compute `graph_coverage`.

---

### AsyncAPIParser

**File:** `scanner/parser/asyncapi.go`
**Targets:** `asyncapi.yaml`, `asyncapi.json`
**Confidence:** authoritative

Parses the AsyncAPI `channels` block. Each channel entry declares `publish` (service produces) or `subscribe` (service consumes). The service name is inferred from the repo entity already in the graph (`_scan_repo` observation lookup); fallback to `info.title`.

**Entities produced:**
- One `event-topic` entity per channel key (e.g. `order.created`).
- Observation on the topic: `schema:<message.name>` if present, `protocol:kafka` if bindings declare it.

**Relations produced:**
- `publishes_event` for each channel where the operation is `publish`.
- `subscribes_event` for each channel where the operation is `subscribe`.

---

### SpringKafkaParser

**File:** `scanner/parser/springkafka.go`
**Targets:** `application.yml`, `application.yaml`, `application.properties`
**Confidence:** inferred

Handles two formats:

**YAML keys scanned:**
```yaml
spring.kafka.producer.topic: order.created
spring.kafka.consumer.topics: payment.approved, fraud.checked
# also nested form:
spring:
  kafka:
    producer:
      topic: order.created
    consumer:
      topics: payment.approved, fraud.checked
```

**Properties keys scanned:**
```
spring.kafka.producer.topic=order.created
spring.kafka.consumer.topics=payment.approved,fraud.checked
```

Comma-separated topic lists are split and produce one relation per topic. Topics that look like `${ENV_VAR}` placeholders are skipped (not resolvable at parse time).

---

### OpenAPIParser

**File:** `scanner/parser/openapi.go`
**Targets:** `openapi.yaml`, `openapi.json`, `swagger.json`, `swagger.yaml`
**Confidence:** authoritative

Extracts:
- `info.title` → entity name (type `api`, already supported)
- `info.version` → observation `version:<v>`
- `servers[].url` → observation `server_url:<url>` (enables future cross-service URL matching)
- Count of paths → observation `paths:<n>`

Produces `exposes_api` relation from the repo's service entity to the API entity.

Does **not** attempt to infer which other services call this API — that requires runtime data outside scope.

---

### ProtoParser

**File:** `scanner/parser/proto.go`
**Targets:** `*.proto`
**Confidence:** authoritative

> **Discovery note:** `*.proto` is not a root-level filename, so `Filenames()` returns `[".proto"]` as a sentinel suffix. `classifyFile` in `scanner/scanner.go` is extended to check `strings.HasSuffix(normalized, p.Filenames()[0])` when the entry starts with `.`, in addition to the existing exact-match loop. This is a minimal, backward-compatible extension to the #13 classification logic.

No external proto parsing library — uses line-by-line scanning to avoid heavy dependencies:

- `service Foo {` → creates entity `Foo` of type `grpc-service`; produces `provides_grpc` from the repo service to `Foo`.
- `import "other/bar.proto"` → extracts `bar` as a candidate grpc-service name; produces `depends_on_grpc` relation (confidence: inferred, because the import may be from an external SDK, not an internal service). Observation `_grpc_import_path:<path>` stored for disambiguation.
- Package declaration → observation `proto_package:<name>`.

Import-based relations are marked `_integration_source:proto-import` (vs `proto` for direct service definitions) so the tool can downgrade their confidence to `inferred`.

---

### K8sServiceParser

**File:** `scanner/parser/k8sintegration.go`
**Targets:** `deploy/**/*.yaml`, `deploy/**/*.yml`
**Confidence:** inferred

> **Discovery note:** Deploy yamls are already scanned by the existing infra scanner via `scanInfraDir`. This parser registers `FileType() = "k8s"` — the same type already emitted by `classifyFile` for K8s manifests. The AutoIndexer's generic loop in `runParsers` will route files of type `"k8s"` through this parser automatically once it's registered. No change to scanner discovery is needed.

Scans environment variable names in Kubernetes `Deployment` and `Pod` specs for patterns:
- `*_SERVICE_HOST` → extract prefix, normalize to lowercase-hyphenated, produce `calls_service` relation.
- `*_SERVICE_URL`, `*_API_URL`, `*_BASE_URL` → same normalization.

Example: `PAYMENT_SERVICE_HOST` → `calls_service` to entity `payment-service` (or `payment` if exact match found in graph).

Normalization: `PAYMENT_SERVICE` → try exact match in graph first, then `payment-service`, then `payment`. If no match found in graph, create a stub entity with `_status:stub` observation — preserves the relation without polluting the graph with noise.

---

## Layer 2: `get_integration_map` Tool

**File:** `tools/get_integration_map.go`

```
Tool: get_integration_map

Description:
  Returns the complete integration topology of a service in a single call:
  which events it publishes and subscribes to, which APIs and gRPC services
  it exposes or depends on, and which services it calls directly. Each entry
  includes a confidence level so the AI agent can distinguish authoritative
  contract declarations from inferred configuration values.
  Use this tool before any architecture, impact analysis, or documentation task
  involving a specific service — it eliminates the need to read raw config files.

Input:
  service  string  (required) — entity name in the graph (e.g. "checkout-service")
  depth    int     (optional, default 1, max 3) — integration hops to include;
                   depth=1 returns direct integrations only;
                   depth=2 includes integrations of integrations

Output (structured JSON):
{
  "service": "checkout-service",
  "publishes": [
    { "topic": "order.created", "schema": "OrderCreatedEvent", "confidence": "authoritative" }
  ],
  "subscribes": [
    { "topic": "payment.approved", "confidence": "inferred" }
  ],
  "exposes_api": [
    { "name": "checkout-api", "version": "2.1.0", "paths": 14, "confidence": "authoritative" }
  ],
  "provides_grpc": [
    { "service": "CheckoutService", "confidence": "authoritative" }
  ],
  "grpc_deps": [
    { "service": "FraudService", "source_repo": "fraud-service", "confidence": "inferred" }
  ],
  "calls": [
    { "service": "payment-service", "confidence": "inferred" }
  ],
  "graph_coverage": "partial"
}
```

**`graph_coverage` values:**

| Value | Meaning |
|---|---|
| `"full"` | At least one `authoritative` source (AsyncAPI or proto) covers all integration directions found |
| `"partial"` | Mix of authoritative and inferred, or some directions have no data |
| `"inferred"` | All relations come from config heuristics only |
| `"none"` | No integration relations found — service may not be scanned yet |

This field is the primary token-saving mechanism: the AI reads one field and decides whether to trust the answer or flag the gap to the user.

---

## Memory Layer

**File:** `memory/integration.go`

New method on `MemoryService`:

```go
type IntegrationMap struct {
    Service     string
    Publishes   []IntegrationEdge
    Subscribes  []IntegrationEdge
    ExposesAPI  []IntegrationEdge
    ProvidesGRPC []IntegrationEdge
    GRPCDeps    []IntegrationEdge
    Calls       []IntegrationEdge
    Coverage    string // "full" | "partial" | "inferred" | "none"
}

type IntegrationEdge struct {
    Target     string
    Schema     string // optional, for event-topic entities
    Version    string // optional, for api entities
    Paths      int    // optional, for api entities
    Confidence string // "authoritative" | "inferred"
    SourceRepo string
}

func (m *MemoryService) GetIntegrationMap(ctx context.Context, service string, depth int) (IntegrationMap, error)
```

Implemented as SQL queries over `db_relations` filtered by the integration relation types. No full graph load — targeted queries per relation type, O(relation_types × depth) queries maximum.

`graph_coverage` is computed from `_integration_source` observations on the service entity after the relations are loaded.

---

## Files Changed

| File | Change |
|---|---|
| `scanner/parser/asyncapi.go` | **New** — `AsyncAPIParser` |
| `scanner/parser/springkafka.go` | **New** — `SpringKafkaParser` |
| `scanner/parser/openapi.go` | **New** — `OpenAPIParser` |
| `scanner/parser/proto.go` | **New** — `ProtoParser` |
| `scanner/parser/k8sintegration.go` | **New** — `K8sServiceParser` |
| `scanner/scanner.go` | Add `*.proto` to `DefaultTargetFiles`; rest already covered by infra scanner |
| `memory/integration.go` | **New** — `GetIntegrationMap` + types |
| `memory/memory.go` | Expose `GetIntegrationMap` on `MemoryService`; add to interface |
| `tools/get_integration_map.go` | **New** — handler + typed args/result |
| `tools/ports.go` | Add `GetIntegrationMap` to `GraphStore` interface |
| `tools/tools.go` | Register `get_integration_map` tool |
| `main.go` | Register 5 new parsers in `parser.Default` |
| `tests/integration_map/integration_map_test.go` | **New** — E2E: populate graph via parsers, assert tool output |
| `AGENTS.md` | Update §7 with new relation types and `get_integration_map` usage |
| `ROADMAP.md` | Add `#15 Integration Topology Discovery` |

---

## Testing Strategy

**Unit — each parser (`scanner/parser/*_test.go`):**
- Happy path: valid file → correct `ParsedFile` with expected entities and relations
- Edge cases: empty topics list, placeholder `${VAR}` topics skipped, malformed YAML returns error
- `FileType()` and `Filenames()` return expected values

**Unit — `memory/integration_test.go`:**
- `GetIntegrationMap` with no relations → returns `coverage: "none"`
- Mix of authoritative + inferred sources → `coverage: "partial"`
- Only authoritative → `coverage: "full"`

**E2E — `tests/integration_map/integration_map_test.go`:**
- Build a small graph with 3 services, 2 topics, 1 gRPC service via mock parsers
- Call `get_integration_map` for each service
- Assert: correct `publishes`, `subscribes`, `grpc_deps`, `calls` entries
- Assert: `graph_coverage` computed correctly
- Assert: `depth=2` traverses one hop further

---

## Non-Goals

- Source code scanning (e.g. detecting `kafkaTemplate.send(...)` in Java source) — too fragile
- Schema Registry integration — out of scope
- Topic versioning or schema evolution tracking — out of scope
- Inferring HTTP callers from access logs or tracing data — out of scope
- Dynamic plugin loading for custom protocol parsers — handled by `#13` already
