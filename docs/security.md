# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| `main` branch | ✅ |
| Latest tagged release | ✅ |
| Older releases | ❌ |

We do not backport security fixes to older unmaintained versions.

---

## Security Model

DocScout-MCP is designed to be safe by default:

**Path-traversal protection**
: The `get_file_content` tool only serves files that were discovered and indexed by the scanner. The AI cannot request arbitrary file paths or read files outside the indexed set.

**STDIO integrity**
: The server communicates via JSON-RPC over `stdio`. No free text is ever written to `stdout`. All diagnostic output goes to `stderr`, making JSON-RPC stream corruption impossible by design.

**GitHub token scope**
: The required Fine-Grained PAT needs only read-only `Contents` and `Metadata` access. No write permissions are ever required.

**Graph integrity**
: Observations are sanitized before storage (empty, too-short, too-long, and duplicate strings are rejected). Mass deletions of more than 10 entities require explicit `confirm: true`.

**Audit log**
: Every graph mutation emits a structured `slog.Info` line to stderr. When `DATABASE_URL` points to a persistent store, mutations are also written to an `audit_events` table with a UUIDv7 primary key (chronological ordering without an extra index). The `query_audit_log` and `get_audit_summary` MCP tools surface this data for governance reviews. Mass deletions (`count > 10`), unknown agents, and error bursts are automatically flagged as risky events.

**Constant-time token comparison**
: The HTTP bearer token (`MCP_HTTP_BEARER_TOKEN`) is compared using `crypto/subtle.ConstantTimeCompare` to prevent timing attacks.

---

## Reporting a Vulnerability

**Do NOT report security vulnerabilities via public GitHub Issues.**

DocScout-MCP handles internal corporate documentation and architecture graphs — we take security seriously.

If you discover a security vulnerability (such as a flaw in the path-traversal mitigation, token leakage, or injection in the graph layer), please report it via:

- **GitHub Private Vulnerability Reporting** (preferred): use the Security tab on the repository
- **Email**: contact the maintainer directly

### What to include

- A description of the vulnerability and its potential impact
- Steps to reproduce
- Any proposed fixes or mitigations

### Response time

We aim to acknowledge reports within **48 hours** and will provide an estimated fix timeline. You will be kept updated throughout.
