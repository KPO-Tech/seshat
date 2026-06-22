#!/usr/bin/env bash
set -euo pipefail

REPO="EngineerProjects/seshat"
BINARY="seshat"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# ── Detect OS / arch ──────────────────────────────────────────────────────────

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "Unsupported OS: $OS" >&2
    echo "Download a binary manually from: https://github.com/$REPO/releases" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

SUFFIX="${os}-${arch}"

# ── Resolve version ───────────────────────────────────────────────────────────

if [ -z "${VERSION:-}" ]; then
  echo "Fetching latest release..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
fi

if [ -z "$VERSION" ]; then
  echo "Could not determine latest version. Set VERSION= explicitly." >&2
  exit 1
fi

echo "Installing seshat $VERSION ($SUFFIX)..."

# ── Download ──────────────────────────────────────────────────────────────────

BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
BIN_NAME="${BINARY}-${SUFFIX}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$BASE_URL/$BIN_NAME" -o "$TMP_DIR/$BINARY"
curl -fsSL "$BASE_URL/SHA256SUMS.txt" -o "$TMP_DIR/SHA256SUMS.txt"

# ── Verify checksum ───────────────────────────────────────────────────────────

(cd "$TMP_DIR" && grep "$BIN_NAME" SHA256SUMS.txt | sha256sum --check --status) || {
  echo "Checksum verification failed." >&2
  exit 1
}

# ── Install ───────────────────────────────────────────────────────────────────

mkdir -p "$INSTALL_DIR"
chmod +x "$TMP_DIR/$BINARY"
mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"

echo ""
echo "seshat $VERSION installed to $INSTALL_DIR/seshat"

# Warn if INSTALL_DIR is not in PATH
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo ""
    echo "  Note: $INSTALL_DIR is not in your PATH."
    echo "  Add this to your shell profile (~/.bashrc, ~/.zshrc, ...):"
    echo ""
    echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    ;;
esac

echo "Run: seshat version"
