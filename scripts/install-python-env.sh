#!/usr/bin/env bash
# install-python-env.sh — bootstrap the Nexus Python environment using uv.
#
# What it does:
#   1. Install uv (Rust-based Python manager) if not already on PATH.
#      uv bundles its own Python runtime — no system Python required.
#   2. Create a venv at $NEXUS_RUNTIME_ROOT/.venv using Python 3.11+.
#   3. Install docling-serve into that venv for document conversion.
#
# Environment variables (all optional):
#   NEXUS_RUNTIME_ROOT   Config/data root (default: ~/.config/nexus-cli on Linux/macOS,
#                        %APPDATA%\nexus-cli on Windows — mirrors Go DefaultConfigDir)
#   NEXUS_CONFIG_DIR     Legacy alias for NEXUS_RUNTIME_ROOT (takes lower priority)
#   DOCLING_EXTRAS       pip extras, e.g. "gpu" → installs docling-serve[gpu]
#   PYTHON_VERSION       Python version for the venv (default: 3.11)
#
# After running:
#   ./scripts/start-docling.sh     — start manually
#   nexus chat                     — Nexus auto-starts docling on launch

set -euo pipefail

# ── Resolve runtime root (same logic as Go DefaultConfigDir) ──────────────────
_default_runtime_root() {
    local os
    os="$(uname -s 2>/dev/null || echo Linux)"
    if [ -n "${XDG_CONFIG_HOME:-}" ]; then
        echo "$XDG_CONFIG_HOME/nexus-cli"
    elif [ "$os" = "Darwin" ] || [ "$os" = "Linux" ]; then
        echo "$HOME/.config/nexus-cli"
    else
        echo "$HOME/.config/nexus-cli"  # fallback for unknown POSIX
    fi
}

NEXUS_RUNTIME_ROOT="${NEXUS_RUNTIME_ROOT:-${NEXUS_CONFIG_DIR:-$(_default_runtime_root)}}"
VENV_DIR="$NEXUS_RUNTIME_ROOT/.venv"
PYTHON_VERSION="${PYTHON_VERSION:-3.11}"
DOCLING_EXTRAS="${DOCLING_EXTRAS:-}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; NC='\033[0m'

info()    { echo -e "${BLUE}[nexus]${NC} $*"; }
success() { echo -e "${GREEN}[nexus]${NC} $*"; }
warn()    { echo -e "${YELLOW}[nexus]${NC} $*"; }
error()   { echo -e "${RED}[nexus]${NC} $*" >&2; }

# ── 1. Install uv if missing ──────────────────────────────────────────────────
if command -v uv &>/dev/null; then
    success "uv already installed: $(uv --version)"
else
    info "uv not found — installing via official installer..."

    if command -v curl &>/dev/null; then
        curl -LsSf https://astral.sh/uv/install.sh | sh
    elif command -v wget &>/dev/null; then
        wget -qO- https://astral.sh/uv/install.sh | sh
    else
        error "Neither curl nor wget found. Install uv manually: https://astral.sh/uv"
        exit 1
    fi

    export PATH="$HOME/.cargo/bin:$HOME/.local/bin:$PATH"

    if ! command -v uv &>/dev/null; then
        error "uv installation succeeded but not found on PATH."
        error "Add \$HOME/.local/bin to your PATH then re-run this script."
        exit 1
    fi
    success "uv installed: $(uv --version)"
fi

# ── 2. Create runtime root structure ─────────────────────────────────────────
mkdir -p "$NEXUS_RUNTIME_ROOT"
info "Runtime root: $NEXUS_RUNTIME_ROOT"

# ── 3. Create the venv ───────────────────────────────────────────────────────
if [ -d "$VENV_DIR" ]; then
    info "Python venv already exists at $VENV_DIR"
else
    info "Creating Python $PYTHON_VERSION venv at $VENV_DIR ..."
    uv venv "$VENV_DIR" --python "$PYTHON_VERSION" --seed
    success "Venv created."
fi

# ── 4. Install docling-serve ─────────────────────────────────────────────────
PACKAGE="docling-serve"
if [ -n "$DOCLING_EXTRAS" ]; then
    PACKAGE="docling-serve[$DOCLING_EXTRAS]"
fi

info "Installing $PACKAGE into $VENV_DIR ..."
uv pip install --python "$VENV_DIR/bin/python" "$PACKAGE"
success "docling-serve installed."

# ── 5. Verify ────────────────────────────────────────────────────────────────
DOCLING_BIN="$VENV_DIR/bin/docling-serve"
if [ ! -x "$DOCLING_BIN" ]; then
    error "docling-serve binary not found at $DOCLING_BIN after installation."
    exit 1
fi

success "Installation complete."
echo ""
echo "  Runtime root: $NEXUS_RUNTIME_ROOT"
echo "  Venv:         $VENV_DIR"
echo "  Binary:       $DOCLING_BIN"
echo ""
echo "  Start docling-serve manually:"
echo "    ./scripts/start-docling.sh"
echo ""
echo "  Or just run nexus — it auto-starts docling on launch."
echo ""
