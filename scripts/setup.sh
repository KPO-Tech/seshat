#!/usr/bin/env bash
# scripts/setup.sh — One-command setup for nexus-engine on Linux and macOS.
#
# What it does:
#   1. Verifies Go 1.21+
#   2. Installs ripgrep (required for glob/grep tools)
#   3. Installs uv (Rust-based Python manager — no system Python needed)
#   4. Creates Python venv + installs docling-serve for document conversion
#   5. Builds nexus and nexus-grpc binaries to bin/
#   6. Installs git pre-commit hooks
#
# Usage:
#   ./scripts/setup.sh
#   DOCLING_EXTRAS=gpu ./scripts/setup.sh    # GPU-accelerated docling
#
# Environment variables:
#   NEXUS_RUNTIME_ROOT   Override data dir (default: ~/.config/nexus-cli)
#   DOCLING_EXTRAS       pip extras for docling-serve (e.g. "gpu")
#   PYTHON_VERSION       Python version for the venv (default: 3.11)
#   SKIP_PYTHON          Set to 1 to skip the Python/docling setup step

set -euo pipefail

OS="$(uname -s)"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ── Runtime root (mirrors pkg/runtimepath DefaultConfigDir logic) ─────────────
if [ -z "${NEXUS_RUNTIME_ROOT:-}" ]; then
    if [ -n "${XDG_CONFIG_HOME:-}" ]; then
        NEXUS_RUNTIME_ROOT="$XDG_CONFIG_HOME/nexus-cli"
    else
        NEXUS_RUNTIME_ROOT="$HOME/.config/nexus-cli"
    fi
fi
export NEXUS_RUNTIME_ROOT

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

ok()   { echo -e "${GREEN}  ✓${NC}  $*"; }
info() { echo -e "${BLUE}  ·${NC}  $*"; }
warn() { echo -e "${YELLOW}  !${NC}  $*"; }
fail() { echo -e "${RED}  ✗${NC}  $*" >&2; exit 1; }
step() { echo -e "\n${BOLD}$*${NC}"; }

# ── 1. Go ─────────────────────────────────────────────────────────────────────
step "Checking Go..."

if ! command -v go &>/dev/null; then
    fail "Go not found. Install Go 1.21+ from: https://go.dev/dl/"
fi

GO_VERSION="$(go version | awk '{print $3}' | sed 's/go//')"
GO_MAJOR="$(echo "$GO_VERSION" | cut -d. -f1)"
GO_MINOR="$(echo "$GO_VERSION" | cut -d. -f2)"
if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 21 ]; }; then
    fail "Go $GO_VERSION found but 1.21+ is required. Update at: https://go.dev/dl/"
fi
ok "Go $GO_VERSION"

# ── 2. ripgrep ────────────────────────────────────────────────────────────────
step "Checking ripgrep..."

if command -v rg &>/dev/null; then
    ok "ripgrep $(rg --version | head -1 | awk '{print $2}')"
else
    info "Installing ripgrep..."
    case "$OS" in
        Darwin)
            if command -v brew &>/dev/null; then
                brew install ripgrep
            else
                fail "Homebrew not found. Install it from https://brew.sh/ then re-run setup."
            fi
            ;;
        Linux)
            if command -v apt-get &>/dev/null; then
                sudo apt-get update -qq && sudo apt-get install -y ripgrep
            elif command -v dnf &>/dev/null; then
                sudo dnf install -y ripgrep
            elif command -v pacman &>/dev/null; then
                sudo pacman -S --noconfirm ripgrep
            elif command -v zypper &>/dev/null; then
                sudo zypper install -y ripgrep
            elif command -v apk &>/dev/null; then
                sudo apk add ripgrep
            else
                fail "Cannot detect package manager. Install ripgrep manually:\n  https://github.com/BurntSushi/ripgrep#installation"
            fi
            ;;
        *)
            warn "Unknown OS '$OS'. Install ripgrep manually: https://github.com/BurntSushi/ripgrep#installation"
            ;;
    esac
    ok "ripgrep installed"
fi

# ── 3. uv ─────────────────────────────────────────────────────────────────────
step "Checking uv (Python manager)..."

if command -v uv &>/dev/null; then
    ok "uv $(uv --version)"
else
    info "Installing uv..."
    if command -v curl &>/dev/null; then
        curl -LsSf https://astral.sh/uv/install.sh | sh
    elif command -v wget &>/dev/null; then
        wget -qO- https://astral.sh/uv/install.sh | sh
    else
        fail "curl or wget required to install uv."
    fi
    # Add uv to PATH for the rest of this session
    export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$PATH"
    if ! command -v uv &>/dev/null; then
        fail "uv installed but not on PATH.\nAdd \$HOME/.local/bin to your PATH then re-run setup."
    fi
    ok "uv $(uv --version)"
fi

# ── 4. Python venv + docling-serve ────────────────────────────────────────────
if [ "${SKIP_PYTHON:-0}" = "1" ]; then
    warn "Skipping Python/docling setup (SKIP_PYTHON=1)"
else
    step "Setting up Python environment..."
    NEXUS_CONFIG_DIR="$NEXUS_RUNTIME_ROOT" \
        bash "$REPO_ROOT/scripts/install-python-env.sh"
fi

# ── 5. Build ──────────────────────────────────────────────────────────────────
step "Building nexus-engine..."

cd "$REPO_ROOT"
mkdir -p bin
go build -o bin/nexus ./cmd/cli
go build -o bin/nexus-grpc ./cmd/grpc
ok "bin/nexus"
ok "bin/nexus-grpc"

# ── 6. Git hooks ──────────────────────────────────────────────────────────────
step "Installing git hooks..."

if [ -d "$REPO_ROOT/.githooks" ] && git -C "$REPO_ROOT" rev-parse --git-dir &>/dev/null; then
    git -C "$REPO_ROOT" config core.hooksPath .githooks
    ok "Git hooks installed from .githooks/"
else
    warn "No .githooks/ directory found — skipping"
fi

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}${GREEN}  Setup complete!${NC}"
echo ""
echo "  Runtime data: $NEXUS_RUNTIME_ROOT"
echo ""
echo "  Add bin/ to your PATH:"
echo "    export PATH=\"\$PATH:$REPO_ROOT/bin\""
echo ""
echo "  Configure a provider:"
echo "    nexus config --provider anthropic --api-key sk-ant-..."
echo ""
echo "  Start chatting:"
echo "    nexus chat"
echo ""
