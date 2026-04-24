# DocScout GitHub Action

The `docscout-action` is a [GitHub Actions composite action](https://docs.github.com/en/actions/creating-actions/creating-a-composite-action) that runs a DocScout knowledge-graph scan in CI and surfaces the results as a GitHub Step Summary and optional PR comment.

## Quick Start

```yaml
# .github/workflows/docscout.yml
name: DocScout Graph Analysis

on:
  pull_request:
  push:
    branches: [main]

jobs:
  graph-analysis:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write  # required for comment_on_pr: 'true'
    steps:
      - uses: actions/checkout@v4

      - uses: doc-scout/mcp-server@main
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          comment_on_pr: 'true'
```

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `github_token` | Yes | — | GitHub token with `repo` read access. Use `${{ secrets.GITHUB_TOKEN }}`. |
| `version` | No | `latest` | DocScout release to install, e.g. `v0.2.0`. Defaults to the latest GitHub release. |
| `comment_on_pr` | No | `false` | Post (or update) a PR comment with graph stats when `true`. |
| `entity_types` | No | `''` | Comma-separated entity types to break down in the summary (e.g. `service,api`). |

## Outputs

| Output | Description |
|--------|-------------|
| `entity_count` | Total number of entities discovered in the graph. |
| `relation_count` | Total number of relations discovered in the graph. |

Use outputs in downstream steps:

```yaml
- uses: doc-scout/mcp-server@main
  id: docscout
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}

- name: Check entity count
  run: echo "Found ${{ steps.docscout.outputs.entity_count }} entities"
```

## Step Summary

Every run writes a Markdown table to the [GitHub Step Summary](https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#adding-a-job-summary):

```
## DocScout Graph Analysis

| Metric | Count |
|--------|-------|
| Entities | 42 |
| Relations | 87 |

Scan completed in 45s for `my-org/my-repo`
```

## PR Comments

When `comment_on_pr: 'true'` is set, the action posts (or updates an existing) comment on the pull request with the same stats table. This requires the `pull-requests: write` permission.

## How It Works

1. **Install** — Downloads the pre-built `docscout-mcp` binary from the GitHub Releases page for the requested version. Falls back to `go install` if no binary is available for the runner's architecture.
2. **Scan** — Starts `docscout-mcp` with `HTTP_ADDR` set, targeting the repository's org. Polls `/healthz` until the initial scan completes (up to 120 seconds).
3. **Report** — Queries the local SQLite database for entity and relation counts, writes the Step Summary, sets output variables, and optionally posts a PR comment.

## Supported Runners

| Runner | Architecture | Status |
|--------|-------------|--------|
| `ubuntu-latest` | `amd64` | Supported |
| `ubuntu-24.04-arm` | `arm64` | Supported |
| `macos-*` | — | Binary not provided; requires Go in PATH |
| `windows-*` | — | Not supported |

## Pinning a Specific Version

For production workflows, pin to a specific tag rather than `@main`:

```yaml
- uses: doc-scout/mcp-server@v0.2.0
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    version: 'v0.2.0'
```

## Permissions

The minimal GitHub token permissions required:

```yaml
permissions:
  contents: read         # scan repo files
  pull-requests: write   # post PR comment (only if comment_on_pr: 'true')
```
