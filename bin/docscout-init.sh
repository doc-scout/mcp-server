#!/usr/bin/env sh
# DocScout-MCP — one-command setup
# Usage: curl -fsSL https://raw.githubusercontent.com/doc-scout/mcp-server/main/bin/docscout-init.sh | sh
set -e

REPO="doc-scout/mcp-server"
BINARY="docscout-mcp"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

echo ""
echo "  DocScout-MCP — quick setup"
echo "  ────────────────────────────────"
echo ""

# ── Step 1: collect inputs ────────────────────────────────────────────────────
printf "GitHub PAT (fine-grained, read-only Contents+Metadata): "
read -r GITHUB_TOKEN

printf "GitHub org or username to scan: "
read -r GITHUB_ORG

if [ -z "$GITHUB_TOKEN" ] || [ -z "$GITHUB_ORG" ]; then
  echo "Error: GITHUB_TOKEN and GITHUB_ORG are required." >&2
  exit 1
fi

# ── Step 2: detect OS/arch ────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
esac

# ── Step 3: download latest binary ───────────────────────────────────────────
LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Warning: could not determine latest release; falling back to go run." >&2
  USE_GORUN=1
fi

if [ -z "$USE_GORUN" ]; then
  ASSET="${BINARY}-${OS}-${ARCH}"
  URL="https://github.com/$REPO/releases/download/$LATEST/${ASSET}"
  mkdir -p "$INSTALL_DIR"
  echo "Downloading $BINARY $LATEST ($OS/$ARCH)…"
  if curl -fsSL "$URL" -o "$INSTALL_DIR/$BINARY"; then
    chmod +x "$INSTALL_DIR/$BINARY"
    BINARY_PATH="$INSTALL_DIR/$BINARY"
    echo "  ✓ installed to $BINARY_PATH"
  else
    echo "  Binary not found for $OS/$ARCH — falling back to go run." >&2
    USE_GORUN=1
  fi
fi

if [ -n "$USE_GORUN" ]; then
  BINARY_PATH="go run github.com/doc-scout/mcp-server@latest"
fi

# ── Step 4: write .env.local ──────────────────────────────────────────────────
ENV_FILE="./.env.local"
cat > "$ENV_FILE" <<EOF
GITHUB_TOKEN=$GITHUB_TOKEN
GITHUB_ORG=$GITHUB_ORG
SCAN_INTERVAL=30m
DATABASE_URL=sqlite://docscout.db
EOF
echo "  ✓ wrote $ENV_FILE"

# ── Step 5: print Claude Desktop config ──────────────────────────────────────
echo ""
echo "  Add this to your claude_desktop_config.json (mcpServers block):"
echo ""
cat <<EOF
{
  "docscout": {
    "command": "$BINARY_PATH",
    "env": {
      "GITHUB_TOKEN": "$GITHUB_TOKEN",
      "GITHUB_ORG": "$GITHUB_ORG",
      "SCAN_INTERVAL": "30m",
      "DATABASE_URL": "sqlite://docscout.db"
    }
  }
}
EOF
echo ""
echo "  ✓ Done! Start the server:"
echo ""
if [ -n "$USE_GORUN" ]; then
  echo "    go run github.com/doc-scout/mcp-server@latest"
else
  echo "    $BINARY_PATH"
fi
echo ""
