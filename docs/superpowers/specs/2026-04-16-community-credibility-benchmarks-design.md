---
title: Community Credibility & Benchmark Suite
date: 2026-04-16
status: approved
---

# Community Credibility & Benchmark Suite — Design Spec

## Problem

DocScout-MCP is functionally complete at v1.0.0 but lacks the external evidence needed to attract production adopters and community contributors. The gap is not features — it is **verifiable proof** that the tool does what it claims: accurate knowledge graphs and measurable token savings for AI clients.

## Goal

Establish technical credibility through two complementary benchmarks:
- **B) Accuracy** — does the knowledge graph correctly reflect what is in the repos?
- **C) Token Efficiency** — how many tokens does DocScout save an AI client vs naive file scanning?

These two metrics, published in the README and docs site, are the primary community adoption driver.

## Success Criteria

- `benchmark/RESULTS.md` committed to the repo with reproducible numbers
- Per-parser F1 score ≥ 0.95 on synthetic corpus
- Average token savings ≥ 70% on canonical question set (theoretical mode)
- Live Claude API run confirms theoretical model within ±15%
- Any contributor can reproduce results with `make benchmark` (no API key required for theoretical)
- README has benchmark badges and a "Why DocScout" comparison table

---

## Section 1: Accuracy Benchmark (Synthetic Corpus)

### What We Measure

Precision, Recall, and F1 for entities and relations extracted from repos, per parser.

- **Precision**: of what DocScout found, how much was correct?
- **Recall**: of what should have been found, how much did DocScout find?
- **F1**: harmonic mean — the single number in the README badge

### Synthetic Corpus

`benchmark/testdata/synthetic-org/` — 20 fake repos as flat directories of mock files. No GitHub calls, no network. Covers every parser:

| Repo | Files | Expected entities/relations |
|------|-------|-----------------------------|
| `billing-service` | `go.mod`, `CODEOWNERS`, `openapi.yaml` | service, team, api, `owns`, `exposes_api` |
| `checkout-service` | `go.mod`, `catalog-info.yaml` | service, `depends_on:billing-service` |
| `payment-worker` | `pom.xml`, `asyncapi.yaml` | service, event-topic, `publishes_event` |
| `frontend-app` | `package.json`, `CODEOWNERS` | service, team, `owns` |
| `auth-service` | `go.mod`, `proto/auth.proto` | service, grpc-service, `provides_grpc` |
| `api-gateway` | `go.mod`, `catalog-info.yaml` | service, `calls_service:auth-service` |
| `infra-team` | `CODEOWNERS` only | team, person entities |
| *(13 more covering edge cases)* | | |

Ground truth is committed as `benchmark/testdata/ground_truth.json` — the exact set of entities, relations, and key observations the graph must contain after a full scan. This file is the contract.

### Implementation

`benchmark/accuracy/accuracy_test.go` — a standard Go test (not `testing.B`):

1. Load synthetic fixtures from embedded `embed.FS`
2. Run parser registry + indexer in-process against fixtures
3. Diff resulting graph against `ground_truth.json`
4. Compute per-parser precision, recall, F1
5. Write results to `benchmark/RESULTS.md` (accuracy section)

```
make benchmark-accuracy
go test ./benchmark/accuracy/... -v
```

No network. No external dependencies. Runs in CI on every PR.

---

## Section 2: Token Efficiency Benchmark

### What We Measure

Tokens consumed to answer a canonical set of 12 questions — WITH DocScout vs WITHOUT (naive baseline).

### Naive Baseline

A second MCP configuration exposing only `list_repos` + `get_file_content` (raw GitHub file access, no graph). The AI must read individual files to answer each question. This is the realistic alternative an engineer would reach for without DocScout — not a strawman.

### Canonical Question Set

Committed as `benchmark/questions.json` (12 questions, immutable across runs for comparability):

```json
[
  "Which services depend on billing-service?",
  "Who owns the checkout service?",
  "What would break if db-postgres goes down?",
  "List all services that expose a gRPC endpoint",
  "Which repos have no CODEOWNERS?",
  "What Go services use pgx directly?",
  "Which teams own more than 3 services?",
  "What events does payment-service publish?",
  "Find the shortest dependency path from api-gateway to auth-service",
  "Which services have no OpenAPI spec?",
  "What is the Go version of the oldest service?",
  "List all services that depend on a deprecated library"
]
```

### Two Measurement Modes

**Theoretical mode** (`--mode theoretical`, default, no API key needed):

For each question, estimates tokens WITHOUT DocScout as:
```
naive_tokens = avg_files_ai_reads_to_answer × avg_tokens_per_file
```
File counts derived from the synthetic corpus (deterministic). Average tokens per file is a pre-computed constant in `benchmark/token/model.go`, derived by sampling 50 representative files from `kubernetes/kubernetes` once and committing the result — not re-sampled at runtime. Produces the README table.

**Live mode** (`--mode live`, requires `ANTHROPIC_API_KEY`):

Runs both MCP configurations against a real org (`--org` flag, defaults to `kubernetes/kubernetes`). Requires `ANTHROPIC_API_KEY` (for Claude API calls) and `GITHUB_TOKEN` with read access to the target org. Records actual `usage.input_tokens + usage.output_tokens` from the Claude API per question. Confirms the theoretical model within ±15%.

### API Key Security (Live Mode)

1. **Env var only** — `ANTHROPIC_API_KEY` read exclusively from environment, never accepted as a CLI flag (avoids `ps aux` and shell history exposure)
2. **Never in output** — the report, stdout, and CI logs never echo the key or any derivative
3. **Opt-in** — `--mode live` must be explicitly passed; default is `theoretical`
4. **Cost guard** — `--dry-run` prints question list and estimated cost before any calls; `--max-questions N` caps spend
5. **Startup validation** — makes one zero-cost `GET /v1/models` call to verify key validity before burning quota
6. **CI scope** — live mode runs only on `workflow_dispatch` or release triggers, never on fork PRs; injects `secrets.ANTHROPIC_API_KEY` via env, never echoed

### Output Table

```
| Question                                  | DocScout | Naive   | Savings |
|-------------------------------------------|----------|---------|---------|
| Which services depend on billing-service? | 312 tok  | 4,800   | 93%     |
| Who owns the checkout service?            | 180 tok  | 2,100   | 91%     |
| ...                                       | ...      | ...     | ...     |
| **Average**                               | ~290 tok | ~3,940  | ~93%    |
```

---

## Section 3: Published Benchmark Report + Documentation

### `benchmark/RESULTS.md`

Committed to the repo. Contains:
- Methodology summary (1 paragraph each for accuracy and token efficiency)
- Accuracy F1 table per parser
- Token efficiency table (theoretical + live columns)
- Corpus description (synthetic org + `kubernetes/kubernetes` at N repos, date of run)
- Reproduction instructions
- `generated_at` timestamp + `docscout_version` stamp

This is the document linked from HN, Reddit, and conference submissions.

### `docs/benchmarks.md`

Same content rendered as a MkDocs Material page. Added to main nav. Shows version stamp so readers know if results are current.

### README Additions

**1. Benchmark badges in hero section:**
```markdown
[![Token Savings](https://img.shields.io/badge/token--savings-93%25-brightgreen)](benchmark/RESULTS.md)
[![Graph Accuracy F1](https://img.shields.io/badge/graph--accuracy-F1%200.97-blue)](benchmark/RESULTS.md)
```

**2. Terminal recording** — replace the text "See It In Action" block with an `asciinema` cast exported as GIF, showing DocScout answering "what breaks if db-postgres goes down?" in ~10 seconds. Hosted in `docs/images/demo.gif`.

**3. "Why DocScout" comparison table:**
```markdown
| Approach           | Accuracy      | Token Cost  | Setup   |
|--------------------|---------------|-------------|---------|
| AI reads files raw | Guesses       | ~4,000/q    | None    |
| Backstage catalog  | High          | Medium      | Heavy   |
| DocScout-MCP       | Verified (F1 0.97) | ~290/q | 5 min   |
```

### `docs/examples/` (new pages)

Three scenario pages replacing sparse tool reference:

- `ownership-queries.md` — "Who owns X? What does team Y own?"
- `impact-analysis.md` — "What breaks if I take down X?"
- `dependency-audit.md` — "Which services use a deprecated library?"

Each page follows: question → tool calls → graph output → AI response. Runnable examples against the synthetic org.

---

## Section 4: `--benchmark` Mode (v1.1.0 Shipped Feature)

### CLI

```
docscout-mcp --benchmark [--org myorg] [--mode theoretical|live] [--output results.md] [--dry-run] [--max-questions N]
```

Built as `benchmark/cmd/main.go`, compiled into the same binary. No separate install.

### Execution Flow

1. Load synthetic fixtures from embedded `embed.FS` (no external files after build)
2. Run accuracy suite in-process → per-parser F1
3. Run token efficiency in chosen mode against `--org` (or synthetic if no org given)
4. Write markdown report to `--output` or stdout

### Package Structure

```
benchmark/
├── cmd/              ← entrypoint
├── accuracy/         ← accuracy runner (pure Go, no network)
├── token/            ← token efficiency runner (theoretical + live)
├── report/           ← markdown report generator
├── testdata/
│   ├── synthetic-org/   ← 20 fake repos as flat files
│   └── ground_truth.json
└── questions.json    ← canonical 12-question corpus
```

---

## Section 5: Report Generation Pipeline

Three ways to generate/update `benchmark/RESULTS.md`:

### 1. Local (any contributor)
```bash
make benchmark        # theoretical mode, no API key needed
make benchmark-live   # live mode, reads ANTHROPIC_API_KEY from env
```

### 2. CI — automated theoretical run on every `main` push
`.github/workflows/benchmark.yml`:
- Runs `make benchmark` (theoretical only, no secrets)
- Commits updated `RESULTS.md` back to `main` if numbers changed
- No API key required — keeps the report always fresh

### 3. CI — manual live run on release
`workflow_dispatch` trigger on the release workflow:
- Injects `secrets.ANTHROPIC_API_KEY` and `secrets.GITHUB_TOKEN` via env (never echoed)
- Runs only when triggered by repo owner (`github.actor == github.repository_owner`)
- Never runs on fork PRs
- Runs `make benchmark-live` against `kubernetes/kubernetes`
- Commits results, then proceeds with release tag
- This generates the "official" number in release notes

### Staleness Guard
`RESULTS.md` contains `generated_at` and `docscout_version`. README badge links to the file. Docs page shows the stamp. If stale, any contributor can re-run and PR the update.

---

## Section 6: ROADMAP.md Additions

New items to append under Future Work:

| # | Item | Summary |
|---|------|---------|
| 20 | **Benchmark Suite** | Accuracy (F1 per parser) + token efficiency benchmarks. Synthetic corpus with ground truth. Theoretical and live Claude API modes. |
| 21 | **`--benchmark` CLI Mode** | Shipped feature: users run against their own org, get a shareable markdown report with accuracy F1 and token savings. |
| 22 | **GitHub Actions Action** | `docscout-action`: run a scan in CI, post graph insights and diff as PR comments. |
| 23 | **LLM Eval Harness** | Answer-quality evaluation using an LLM judge (beyond token counting). Measures correctness of AI responses, not just cost. |
| 24 | **OpenTelemetry Traces** | Distributed tracing for production multi-tenant deployments. Span per tool call, per scan, per indexer phase. |

---

## Out of Scope

- RBAC (already in ROADMAP as item 4)
- Multi-cloud adapters (already in ROADMAP as item 5)
- Benchmark results hosted on external services (Codecov, etc.) — `RESULTS.md` in-repo is sufficient
- Automated GIF/video recording in CI — recorded once manually per major release
