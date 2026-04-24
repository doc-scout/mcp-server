# Benchmarks

This page reports DocScout-MCP's accuracy and token efficiency, measured against a [synthetic corpus](https://github.com/doc-scout/mcp-server/tree/main/benchmark/testdata) with committed ground truth.

## Reproducing Results

```bash
# Theoretical (no API key needed)
make benchmark

# Live (requires ANTHROPIC_API_KEY)
make benchmark-live

# Against your own GitHub org
./docscout-mcp --benchmark --org my-org --token $GITHUB_TOKEN
```

For the full methodology, see the [design spec](https://github.com/doc-scout/mcp-server/blob/main/docs/superpowers/specs/2026-04-16-community-credibility-benchmarks-design.md).

---

> Reproduce: `make benchmark` (theoretical) or `make benchmark-live` (requires `ANTHROPIC_API_KEY`)

---

## Accuracy (Synthetic Corpus)

Tests whether each parser correctly extracts entities, relations, and observations from known fixtures.
The corpus covers 15 ground-truth cases across 6 services (`auth-service`, `billing-service`, `checkout-service`, `frontend-app`, `payment-worker`, `notification-service`).

| Parser         | Precision | Recall | F1       | TP  | FP  | FN  |
| -------------- | --------- | ------ | -------- | --- | --- | --- |
| `asyncapi`     | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `catalog-info` | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `codeowners`   | 1.00      | 1.00   | **1.00** | 6   | 0   | 0   |
| `gomod`        | 1.00      | 1.00   | **1.00** | 12  | 0   | 0   |
| `k8s`          | 1.00      | 1.00   | **1.00** | 2   | 0   | 0   |
| `openapi`      | 1.00      | 1.00   | **1.00** | 5   | 0   | 0   |
| `packagejson`  | 1.00      | 1.00   | **1.00** | 7   | 0   | 0   |
| `pomxml`       | 1.00      | 1.00   | **1.00** | 5   | 0   | 0   |
| `proto`        | 1.00      | 1.00   | **1.00** | 4   | 0   | 0   |
| `spring-kafka` | 1.00      | 1.00   | **1.00** | 3   | 0   | 0   |
| **overall**    | 1.00      | 1.00   | **1.00** | 52  | 0   | 0   |

## Relation Confidence Scores

Every relation in the knowledge graph carries a `confidence` tag that reflects how it was derived:

| Confidence      | Source                                    | Example                          |
| --------------- | ----------------------------------------- | -------------------------------- |
| `authoritative` | Explicit contract file (AsyncAPI, OpenAPI, Proto, go.mod, catalog-info) | `payment-worker → payment.completed` via asyncapi.yaml |
| `inferred`      | Config heuristic (Spring Kafka, K8s env vars) | `checkout-service → billing-service` via K8s `SERVICE_URL` env var |
| `ambiguous`     | Caller-supplied; provenance unknown       | Manually created via `create_relations` |

The `traverse_graph` tool returns edges with their confidence level in the `edges` array. The `get_integration_map` tool surfaces an overall `graph_coverage` field: `"full"`, `"partial"`, `"inferred"`, or `"none"`.

## Token Efficiency (Theoretical Model)

Estimated tokens consumed per question: DocScout vs naive file-by-file reading.
Naive baseline assumes an AI reads each relevant file individually from GitHub.

| #   | Question                                           | DocScout | Naive     | Savings   |
| --- | -------------------------------------------------- | -------- | --------- | --------- |
| 1   | Which services depend on billing-service?          | 320      | 27705     | **98.8%** |
| 2   | Who owns the checkout service?                     | 180      | 14776     | **98.8%** |
| 3   | What would break if database goes down?            | 450      | 36940     | **98.8%** |
| 4   | List all services that expose a gRPC endpoint      | 280      | 46175     | **99.4%** |
| 5   | Which repos have no CODEOWNERS?                    | 210      | 36940     | **99.4%** |
| 6   | What Go services depend on billing-service dire... | 300      | 22164     | **98.6%** |
| 7   | Which teams own more than one service?             | 240      | 18470     | **98.7%** |
| 8   | What events does payment-worker publish?           | 190      | 9235      | **97.9%** |
| 9   | Find the shortest dependency path from checkout... | 380      | 33246     | **98.9%** |
| 10  | Which services have no OpenAPI spec?               | 220      | 36940     | **99.4%** |
| 11  | What is the Go version of billing-service?         | 150      | 18470     | **99.2%** |
| 12  | List all services that depend on kafka-client      | 290      | 27705     | **99.0%** |
| —   | **Average**                                        | **267**  | **27397** | **99.0%** |

## Running Against Your Own Org (`--org`)

```bash
./docscout-mcp --benchmark \
  --org my-github-org \
  --token "$GITHUB_TOKEN" \
  --org-timeout 30m \
  --output benchmark/MY_ORG_RESULTS.md
```

This performs a full live scan of `my-github-org`, populates a temporary SQLite database, and appends an **Org Scan Stats** section to the report with:

- Repos scanned and scan duration
- Entity count broken down by type (`service`, `api`, `team`, `person`, `event-topic`, `grpc-service`)
- Relation count broken down by confidence (`authoritative` vs `inferred`)

The `--org-timeout` flag (default 30 minutes) sets a hard deadline on the scan. `--token` falls back to the `GITHUB_TOKEN` environment variable if omitted.
