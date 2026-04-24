#!/usr/bin/env bash
# install-docscout.sh — Download or build the docscout-mcp binary for GitHub Actions.
# Supports Linux amd64 and arm64 (the only architectures used by GitHub-hosted runners).
set -euo pipefail

REPO="doc-scout/mcp-server"
BINARY="docscout-mcp"
INSTALL_DIR="/usr/local/bin"

# ── Detect architecture ───────────────────────────────────────────────────────
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)          ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH — falling back to go install" >&2
    ARCH=""
    ;;
esac

# ── Resolve version ───────────────────────────────────────────────────────────
VERSION="${DOCSCOUT_VERSION:-latest}"

if [ "$VERSION" = "latest" ]; then
  echo "Resolving latest DocScout release..." >&2
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"([^"]+)".*/\1/')" || true
  if [ -z "$VERSION" ]; then
    echo "Warning: could not resolve latest version from GitHub API — falling back to go install" >&2
    ARCH=""
  else
    echo "Resolved latest version: $VERSION" >&2
  fi
fi

# ── Attempt binary download ───────────────────────────────────────────────────
DOWNLOADED=0
if [ -n "$ARCH" ]; then
  ASSET="${BINARY}-linux-${ARCH}"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
  echo "Downloading ${BINARY} ${VERSION} (linux/${ARCH}) from:" >&2
  echo "  ${URL}" >&2

  if curl -fsSL "$URL" -o "/tmp/${BINARY}"; then
    chmod +x "/tmp/${BINARY}"
    # Try to install system-wide; fall back to ~/.local/bin
    if mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}" 2>/dev/null; then
      echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}" >&2
    else
      LOCAL_BIN="${HOME}/.local/bin"
      mkdir -p "$LOCAL_BIN"
      mv "/tmp/${BINARY}" "${LOCAL_BIN}/${BINARY}"
      echo "Installed ${BINARY} to ${LOCAL_BIN}/${BINARY}" >&2
      echo "${LOCAL_BIN}" >> "$GITHUB_PATH"
    fi
    DOWNLOADED=1
  else
    echo "Binary download failed for ${ASSET} — falling back to go install" >&2
  fi
fi

# ── Fallback: go install ──────────────────────────────────────────────────────
if [ "$DOWNLOADED" -eq 0 ]; then
  if ! command -v go &>/dev/null; then
    echo "Error: 'go' is not available and binary download failed." >&2
    echo "Add 'uses: actions/setup-go@v5' before this action, or use a released version." >&2
    exit 1
  fi

  GO_REF="github.com/${REPO}@latest"
  echo "Installing via: go install ${GO_REF}" >&2
  GOBIN="${INSTALL_DIR}" go install "${GO_REF}" 2>/dev/null || {
    # Non-root fallback
    GOBIN="${HOME}/go/bin"
    go install "${GO_REF}"
    echo "${GOBIN}" >> "$GITHUB_PATH"
    echo "Installed ${BINARY} to ${GOBIN}/" >&2
  }
fi

echo "DocScout installation complete." >&2
