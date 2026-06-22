#!/usr/bin/env bash
# install.sh — Seshat end-user installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/EngineerProjects/seshat/main/scripts/install.sh | bash
#
# Options (env vars):
#   VERSION=v0.1.0       Install a specific version (default: latest)
#   INSTALL_DIR=...      Binary destination (default: ~/.local/bin)
#   NO_PYTHON=1          Skip uv + docling-serve setup
#   DOCLING_EXTRAS=gpu   Install docling-serve[gpu] instead of the base package
#   PYTHON_VERSION=3.12  Python version for the docling venv (default: 3.11)
#
# Developer / SDK usage (no installer needed):
#   go install github.com/EngineerProjects/seshat/cmd/cli@latest
#   go get     github.com/EngineerProjects/seshat@latest   # in your go.mod

set -euo pipefail

# Portable SHA256 check: sha256sum (Linux) or shasum -a 256 (macOS)
_sha256check() {
    if command -v sha256sum &>/dev/null; then
        sha256sum --check --status
    elif command -v shasum &>/dev/null; then
        shasum -a 256 --check --status
    else
        error "No SHA256 tool found (sha256sum or shasum)" ; exit 1
    fi
}

REPO="EngineerProjects/seshat"
BINARY="seshat"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
NO_PYTHON="${NO_PYTHON:-0}"
PYTHON_VERSION="${PYTHON_VERSION:-3.11}"
DOCLING_EXTRAS="${DOCLING_EXTRAS:-}"

# ── Colors ────────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
    RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
    BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'
else
    RED=''; GREEN=''; YELLOW=''; BLUE=''; BOLD=''; NC=''
fi

info()    { echo -e "${BLUE}[seshat]${NC} $*"; }
success() { echo -e "${GREEN}[seshat]${NC} $*"; }
warn()    { echo -e "${YELLOW}[seshat]${NC} $*"; }
error()   { echo -e "${RED}[seshat]${NC} $*" >&2; }
step()    { echo -e "\n${BOLD}▸ $*${NC}"; }

# ── Runtime root (mirrors Go DefaultConfigDir logic) ──────────────────────────
_runtime_root() {
    if [ -n "${SESHAT_RUNTIME_ROOT:-}" ]; then echo "$SESHAT_RUNTIME_ROOT"; return; fi
    if [ -n "${XDG_CONFIG_HOME:-}" ]; then echo "$XDG_CONFIG_HOME/seshat-cli"; return; fi
    echo "$HOME/.config/seshat-cli"
}
RUNTIME_ROOT="$(_runtime_root)"

# ── Detect OS / arch ──────────────────────────────────────────────────────────
step "Detecting platform"

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *)
        error "Unsupported OS: $OS"
        error "Download a binary manually: https://github.com/$REPO/releases"
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64|amd64)  arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
        error "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

SUFFIX="${os}-${arch}"
info "Platform: $SUFFIX"

# ── Resolve version ───────────────────────────────────────────────────────────
step "Resolving version"

if [ -z "${VERSION:-}" ]; then
    info "Fetching latest release..."
    _dl() { command -v curl &>/dev/null && curl -fsSL "$1" || wget -qO- "$1"; }
    VERSION="$(_dl "https://api.github.com/repos/$REPO/releases/latest" \
        | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
fi

[ -z "$VERSION" ] && { error "Could not resolve version. Set VERSION=vX.Y.Z explicitly."; exit 1; }
info "Version: $VERSION"

# ── Download binary ───────────────────────────────────────────────────────────
step "Downloading seshat $VERSION"

BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
BIN_ASSET="${BINARY}-${SUFFIX}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

_dl() { command -v curl &>/dev/null && curl -fsSL "$1" -o "$2" || wget -qO "$2" "$1"; }

info "Binary:    $BIN_ASSET"
_dl "$BASE_URL/$BIN_ASSET"       "$TMP_DIR/$BINARY"
_dl "$BASE_URL/SHA256SUMS.txt"   "$TMP_DIR/SHA256SUMS.txt"

# ── Verify checksum ───────────────────────────────────────────────────────────
step "Verifying checksum"

(cd "$TMP_DIR" && grep "$BIN_ASSET" SHA256SUMS.txt | _sha256check) \
    || { error "Checksum verification failed — aborting."; exit 1; }
success "Checksum OK"

# ── Install binary ────────────────────────────────────────────────────────────
step "Installing binary"

mkdir -p "$INSTALL_DIR"
chmod +x "$TMP_DIR/$BINARY"
mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
success "Installed: $INSTALL_DIR/$BINARY"

# ── Add to PATH in shell profile ──────────────────────────────────────────────
step "Configuring PATH"

_add_to_path() {
    local profile="$1"
    local line='export PATH="$HOME/.local/bin:$PATH"'
    if [ -f "$profile" ] && grep -qF '.local/bin' "$profile"; then
        info "$profile already exports ~/.local/bin — skipping"
    else
        echo "" >> "$profile"
        echo "# Added by seshat installer" >> "$profile"
        echo "$line" >> "$profile"
        success "Added PATH export to $profile"
    fi
}

case ":$PATH:" in
    *":$INSTALL_DIR:"*)
        success "$INSTALL_DIR already in PATH"
        RELOAD_NEEDED=0
        ;;
    *)
        warn "$INSTALL_DIR not in PATH — adding to shell profile"
        RELOAD_NEEDED=1

        # Fish shell
        if [ -n "${FISH_VERSION:-}" ] || echo "${SHELL:-}" | grep -q fish; then
            FISH_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/fish"
            mkdir -p "$FISH_DIR"
            FISH_CONF="$FISH_DIR/config.fish"
            if grep -qF '.local/bin' "$FISH_CONF" 2>/dev/null; then
                info "$FISH_CONF already exports ~/.local/bin — skipping"
            else
                echo "" >> "$FISH_CONF"
                echo "# Added by seshat installer" >> "$FISH_CONF"
                echo 'fish_add_path $HOME/.local/bin' >> "$FISH_CONF"
                success "Added PATH to $FISH_CONF"
            fi
        else
            # Bash
            [ -f "$HOME/.bashrc" ] && _add_to_path "$HOME/.bashrc"
            # Zsh
            [ -f "$HOME/.zshrc"  ] && _add_to_path "$HOME/.zshrc"
            # Fallback: .profile
            if [ ! -f "$HOME/.bashrc" ] && [ ! -f "$HOME/.zshrc" ]; then
                _add_to_path "$HOME/.profile"
            fi
        fi
        ;;
esac

# ── Python / docling setup via seshat setup ──────────────────────────────────
# Use the freshly installed binary so the logic lives in one place.
SESHAT_BIN="$INSTALL_DIR/seshat"
export PATH="$INSTALL_DIR:$HOME/.local/bin:$HOME/.cargo/bin:$PATH"

if [ "$NO_PYTHON" = "1" ]; then
    warn "Skipping Python setup (NO_PYTHON=1)"
    warn "Run 'seshat setup' later to enable document processing."
else
    step "Setting up Python environment (uv + docling-serve)"

    SETUP_ARGS=""
    [ -n "$PYTHON_VERSION" ] && SETUP_ARGS="$SETUP_ARGS --python $PYTHON_VERSION"
    [ -n "$DOCLING_EXTRAS" ] && SETUP_ARGS="$SETUP_ARGS --extras $DOCLING_EXTRAS"

    # shellcheck disable=SC2086
    SESHAT_RUNTIME_ROOT="$RUNTIME_ROOT" "$SESHAT_BIN" setup $SETUP_ARGS
fi

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}✓ Seshat $VERSION installed successfully${NC}"
echo ""
echo "  Binary:  $INSTALL_DIR/seshat"
echo "  Runtime: $RUNTIME_ROOT"
echo "           (DB + sessions are created automatically on first run)"
echo ""

if [ "${RELOAD_NEEDED:-0}" = "1" ]; then
    echo -e "${YELLOW}  Reload your shell to activate PATH:${NC}"
    echo "    source ~/.bashrc   # or ~/.zshrc / open a new terminal"
    echo ""
fi

echo "  Get started:"
echo "    seshat config     # configure your AI provider + API key"
echo "    seshat chat       # start chatting"
echo ""
echo "  Docs: https://github.com/$REPO"
echo ""
