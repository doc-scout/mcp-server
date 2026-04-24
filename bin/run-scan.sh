#!/usr/bin/env bash
# run-scan.sh — Run a single DocScout scan against the current GitHub repository,
# emit a GitHub Step Summary, set output variables, and optionally post a PR comment.
set -euo pipefail

# ── Validate required inputs ──────────────────────────────────────────────────
if [ -z "${GITHUB_TOKEN:-}" ]; then
  echo "Error: GITHUB_TOKEN is not set." >&2
  exit 1
fi

if [ -z "${GITHUB_REPOSITORY:-}" ]; then
  echo "Error: GITHUB_REPOSITORY is not set." >&2
  exit 1
fi

GITHUB_ORG="$(echo "$GITHUB_REPOSITORY" | cut -d/ -f1)"
COMMENT_ON_PR="${COMMENT_ON_PR:-false}"
ENTITY_TYPES="${ENTITY_TYPES:-}"
PR_NUMBER="${PR_NUMBER:-}"

# ── Prepare temp database ─────────────────────────────────────────────────────
TMPDB="$(mktemp /tmp/docscout-XXXXXX.db)"
SCAN_LOG="/tmp/docscout-scan.log"
SCAN_PORT=18765
HTTP_BASE="http://127.0.0.1:${SCAN_PORT}"

cleanup() {
  if [ -n "${SCAN_PID:-}" ] && kill -0 "$SCAN_PID" 2>/dev/null; then
    kill "$SCAN_PID" 2>/dev/null || true
    wait "$SCAN_PID" 2>/dev/null || true
  fi
  rm -f "$TMPDB"
}
trap cleanup EXIT

echo "Starting DocScout scan for ${GITHUB_REPOSITORY}..." >&2

# ── Start DocScout server in the background ───────────────────────────────────
# SCAN_INTERVAL=999h ensures the background server only does one scan then idles.
DATABASE_URL="sqlite://${TMPDB}" \
  GITHUB_TOKEN="${GITHUB_TOKEN}" \
  GITHUB_ORG="${GITHUB_ORG}" \
  SCAN_INTERVAL="999h" \
  HTTP_ADDR="127.0.0.1:${SCAN_PORT}" \
  docscout-mcp >"$SCAN_LOG" 2>&1 &
SCAN_PID=$!

echo "DocScout server started (PID ${SCAN_PID}), waiting for scan to complete..." >&2

# ── Poll /healthz until status=ok or timeout (120 s) ─────────────────────────
TIMEOUT=120
ELAPSED=0
READY=0

while [ "$ELAPSED" -lt "$TIMEOUT" ]; do
  sleep 2
  ELAPSED=$((ELAPSED + 2))

  HTTP_STATUS="$(curl -fsSL --max-time 3 "${HTTP_BASE}/healthz" 2>/dev/null || true)"
  if [ -z "$HTTP_STATUS" ]; then
    # Server not yet accepting connections
    if ! kill -0 "$SCAN_PID" 2>/dev/null; then
      echo "Error: DocScout process exited unexpectedly. Logs:" >&2
      cat "$SCAN_LOG" >&2
      exit 1
    fi
    continue
  fi

  STATUS_FIELD="$(echo "$HTTP_STATUS" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)"
  if [ "$STATUS_FIELD" = "ok" ]; then
    READY=1
    echo "Scan completed in ${ELAPSED}s." >&2
    break
  fi

  echo "  Waiting... (${ELAPSED}s elapsed, status=${STATUS_FIELD:-unknown})" >&2
done

if [ "$READY" -eq 0 ]; then
  echo "Warning: Scan did not report ready within ${TIMEOUT}s — reading partial results." >&2
fi

# ── Query entity and relation counts via sqlite3 ──────────────────────────────
# GORM pluralises snake_case struct names: dbEntity -> db_entities, dbRelation -> db_relations
if ! command -v sqlite3 &>/dev/null; then
  echo "Warning: sqlite3 not found — installing..." >&2
  apt-get install -y -qq sqlite3 >/dev/null 2>&1 || true
fi

ENTITY_COUNT=0
RELATION_COUNT=0

if command -v sqlite3 &>/dev/null && [ -f "$TMPDB" ]; then
  ENTITY_COUNT="$(sqlite3 "$TMPDB" "SELECT COUNT(*) FROM db_entities;" 2>/dev/null || echo 0)"
  RELATION_COUNT="$(sqlite3 "$TMPDB" "SELECT COUNT(*) FROM db_relations;" 2>/dev/null || echo 0)"
else
  # Fallback: read from /healthz JSON (only has entity count)
  HEALTHZ="$(curl -fsSL --max-time 3 "${HTTP_BASE}/healthz" 2>/dev/null || true)"
  if [ -n "$HEALTHZ" ]; then
    ENTITY_COUNT="$(echo "$HEALTHZ" | grep -o '"entities":[0-9]*' | cut -d: -f2 || echo 0)"
  fi
fi

echo "Results — Entities: ${ENTITY_COUNT}, Relations: ${RELATION_COUNT}" >&2

# ── Build highlighted entity types section (optional) ─────────────────────────
ENTITY_TYPE_SECTION=""
if [ -n "$ENTITY_TYPES" ] && command -v sqlite3 &>/dev/null && [ -f "$TMPDB" ]; then
  ENTITY_TYPE_SECTION="\n### Entity Breakdown\n\n| Type | Count |\n|------|-------|\n"
  IFS=',' read -ra TYPES <<< "$ENTITY_TYPES"
  for TYPE in "${TYPES[@]}"; do
    TYPE="$(echo "$TYPE" | tr -d '[:space:]')"
    COUNT="$(sqlite3 "$TMPDB" "SELECT COUNT(*) FROM db_entities WHERE entity_type = '${TYPE}';" 2>/dev/null || echo 0)"
    ENTITY_TYPE_SECTION="${ENTITY_TYPE_SECTION}| \`${TYPE}\` | ${COUNT} |\n"
  done
fi

# ── Write GitHub Step Summary ─────────────────────────────────────────────────
{
  echo "## DocScout Graph Analysis"
  echo ""
  echo "| Metric | Count |"
  echo "|--------|-------|"
  echo "| Entities | ${ENTITY_COUNT} |"
  echo "| Relations | ${RELATION_COUNT} |"
  echo ""
  echo "Scan completed in ${ELAPSED}s for \`${GITHUB_REPOSITORY}\`"
  if [ -n "$ENTITY_TYPES" ]; then
    printf "%b" "${ENTITY_TYPE_SECTION}"
  fi
} >> "${GITHUB_STEP_SUMMARY}"

# ── Set output variables ──────────────────────────────────────────────────────
echo "entity_count=${ENTITY_COUNT}" >> "${GITHUB_OUTPUT}"
echo "relation_count=${RELATION_COUNT}" >> "${GITHUB_OUTPUT}"

# ── Optional PR comment ───────────────────────────────────────────────────────
if [ "${COMMENT_ON_PR}" = "true" ] && [ -n "${PR_NUMBER}" ]; then
  COMMENT_BODY="## DocScout Graph Analysis

| Metric | Count |
|--------|-------|
| Entities | ${ENTITY_COUNT} |
| Relations | ${RELATION_COUNT} |

Scan completed in ${ELAPSED}s for \`${GITHUB_REPOSITORY}\`"

  if [ -n "$ENTITY_TYPES" ]; then
    COMMENT_BODY="${COMMENT_BODY}

$(printf "%b" "${ENTITY_TYPE_SECTION}")"
  fi

  if command -v gh &>/dev/null; then
    echo "Posting PR comment on PR #${PR_NUMBER}..." >&2
    # Try to update an existing DocScout comment; create new if none exists.
    gh pr comment "${PR_NUMBER}" \
      --body "${COMMENT_BODY}" \
      --edit-last 2>/dev/null \
      || gh pr comment "${PR_NUMBER}" --body "${COMMENT_BODY}"
  else
    echo "Warning: 'gh' CLI not found — skipping PR comment." >&2
  fi
fi

echo "DocScout action completed." >&2
