# Contributing

Thank you for your interest in contributing to DocScout-MCP!

**Full contributing guide:** [doc-scout.github.io/mcp-server/contributing/](https://doc-scout.github.io/mcp-server/contributing/)

## Quick Start

```bash
git clone https://github.com/doc-scout/mcp-server
cd mcp-server
go mod download
go test ./...
```

Read [`AGENTS.md`](AGENTS.md) and [`docs/DEVELOPMENT_GUIDELINES.md`](docs/DEVELOPMENT_GUIDELINES.md) before submitting a PR.

For security vulnerabilities, see [`docs/security.md`](docs/security.md) — do not open a public issue.

## Add a Corpus Example

The benchmark accuracy suite lives in `benchmark/testdata/`. Adding a new fixture helps validate parsers against real-world patterns.

**Steps:**

1. Create a new service directory under `benchmark/testdata/synthetic-org/<service-name>/`.
2. Add one or more manifest files that exercise a parser (e.g. `package.json`, `application.yml`, `go.mod`, `CODEOWNERS`, `asyncapi.yaml`, etc.).
3. Append the corresponding test cases to `benchmark/testdata/ground_truth.json`. Each case needs:
   - `id` — unique snake_case string (e.g. `"myservice-packagejson"`)
   - `parser` — the `FileType()` key of the parser under test
   - `input_file` — path relative to `benchmark/testdata/`
   - `expected_entity_name` / `expected_entity_type` — leave empty `""` for CODEOWNERS-style cases that only produce aux entities
   - `expected_obs_subset` — observations the parser must emit (partial match; extra observations are fine)
   - `expected_rels` — relations that must be present (partial match)
   - `expected_aux` — aux entities that must be present (partial match)
4. Run `go test ./benchmark/...` to verify the new cases pass.
5. Run `mise run format` to normalise import ordering before opening a PR.
