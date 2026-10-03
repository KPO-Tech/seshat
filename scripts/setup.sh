#!/usr/bin/env bash
# scripts/setup.sh — One-command setup for Seshat on Linux and macOS.
#
# What it does:
#   1. Verifies Go 1.21+
#   2. Installs ripgrep (required for glob/grep tools)
#   3. Builds seshat and seshat-grpc binaries to bin/
#   4. Installs git pre-commit hooks
#
# Usage:
#   ./scripts/setup.sh
#
# Environment variables:
#   SESHAT_RUNTIME_ROOT   Override data dir (default: ~/.config/seshat-cli)

set -euo pipefail

OS="$(uname -s)"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ── Runtime root (mirrors pkg/runtimepath DefaultConfigDir logic) ─────────────
if [ -z "${SESHAT_RUNTIME_ROOT:-}" ]; then
    if [ -n "${XDG_CONFIG_HOME:-}" ]; then
        SESHAT_RUNTIME_ROOT="$XDG_CONFIG_HOME/seshat-cli"
    else
        SESHAT_RUNTIME_ROOT="$HOME/.config/seshat-cli"
    fi
fi
export SESHAT_RUNTIME_ROOT

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
        MSYS*|MINGW*|CYGWIN*)
            # Git Bash on Windows - `uname -s` reports something like
            # MSYS_NT-10.0-xxxxx / MINGW64_NT-... here, not a real POSIX
            # distro, so apt/brew/etc never apply. Go through the same
            # Windows package managers setup.ps1 uses.
            if command -v winget.exe &>/dev/null; then
                winget.exe install --id BurntSushi.ripgrep.MSVC --silent --accept-source-agreements --accept-package-agreements
            elif command -v scoop &>/dev/null; then
                scoop install ripgrep
            elif command -v choco.exe &>/dev/null; then
                choco.exe install ripgrep -y
            else
                warn "No winget/scoop/choco found. Install ripgrep manually: https://github.com/BurntSushi/ripgrep/releases (or run scripts\\setup.ps1 from PowerShell instead)."
            fi
            ;;
        *)
            warn "Unknown OS '$OS'. Install ripgrep manually: https://github.com/BurntSushi/ripgrep#installation"
            ;;
    esac
    # Re-check rather than assuming the branch above succeeded - a warn-only
    # fallback (unknown OS, no package manager found) used to still print
    # "ripgrep installed" unconditionally right after it, which was wrong.
    if command -v rg &>/dev/null; then
        ok "ripgrep installed"
    else
        warn "ripgrep still not on PATH. If it just installed via winget, open a new shell (PATH needs a refresh) and re-run setup; otherwise install it manually."
    fi
fi

# ── 3. Build ──────────────────────────────────────────────────────────────────
step "Building Seshat..."

cd "$REPO_ROOT"
mkdir -p bin
go build -o bin/seshat ./cmd/cli
go build -o bin/seshat-grpc ./cmd/grpc
ok "bin/seshat"
ok "bin/seshat-grpc"

# ── 4. Git hooks ──────────────────────────────────────────────────────────────
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
echo "  Runtime data: $SESHAT_RUNTIME_ROOT"
echo ""
echo "  Add bin/ to your PATH:"
echo "    export PATH=\"\$PATH:$REPO_ROOT/bin\""
echo ""
echo "  Configure a provider:"
echo "    seshat config --provider anthropic --api-key sk-ant-..."
echo ""
echo "  Start chatting:"
echo "    seshat chat"
echo ""
