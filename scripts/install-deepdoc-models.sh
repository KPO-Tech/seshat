#!/usr/bin/env bash
# install-deepdoc-models.sh — fetch the ONNX models pkg/nativedoc needs for
# native OCR/layout/table-structure recognition (det.ort, rec.ort,
# layout.ort, tsr.ort, ocr.res), from the InfiniFlow/deepdoc HuggingFace
# repository (Apache-2.0 — see docs/issues/document-intelligence-roadmap.md
# Phase 0 for the full license audit).
#
# This mirrors install-python-env.sh's docling-serve provisioning: same
# runtime-root resolution, same idempotent/re-runnable shape. Unlike
# docling-serve, nativedoc is opt-in (see pkg/nativedoc's own doc comment) -
# running this script does nothing on its own; a caller still has to pass
# the resulting directory to nativedoc.New(...).
#
# Environment variables (all optional):
#   SESHAT_RUNTIME_ROOT   Config/data root (default: ~/.config/seshat on Linux/macOS)
#   SESHAT_CONFIG_DIR     Alias for SESHAT_RUNTIME_ROOT (takes lower priority)
#
# After running, the model directory is:
#   $SESHAT_RUNTIME_ROOT/models/deepdoc
# (see pkg/runtimepath.DeepDocModelsDir - keep this script and that function
# in agreement if either changes.)

set -euo pipefail

# ── Resolve runtime root (same logic as pkg/runtimepath.ResolveRoot) ──────────
_default_runtime_root() {
    local os
    os="$(uname -s 2>/dev/null || echo Linux)"
    if [ -n "${XDG_CONFIG_HOME:-}" ]; then
        echo "$XDG_CONFIG_HOME/seshat"
    elif [ "$os" = "Darwin" ] || [ "$os" = "Linux" ]; then
        echo "$HOME/.config/seshat"
    else
        echo "$HOME/.config/seshat"  # fallback for unknown POSIX
    fi
}

SESHAT_RUNTIME_ROOT="${SESHAT_RUNTIME_ROOT:-${SESHAT_CONFIG_DIR:-$(_default_runtime_root)}}"
MODEL_DIR="$SESHAT_RUNTIME_ROOT/models/deepdoc"
HF_REPO="InfiniFlow/deepdoc"
HF_BASE="https://huggingface.co/${HF_REPO}/resolve/main"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; NC='\033[0m'

info()    { echo -e "${BLUE}[seshat]${NC} $*"; }
success() { echo -e "${GREEN}[seshat]${NC} $*"; }
warn()    { echo -e "${YELLOW}[seshat]${NC} $*"; }
error()   { echo -e "${RED}[seshat]${NC} $*" >&2; }

mkdir -p "$MODEL_DIR"
info "Model directory: $MODEL_DIR"

# ── Fetch each model file, skipping ones already present ─────────────────────
# Sizes are approximate sanity checks (HuggingFace occasionally serves a
# truncated/error response as 200 OK) - a file smaller than 1KB almost
# certainly isn't a real weight file, so it's removed and re-fetched.
FILES="det.ort rec.ort layout.ort tsr.ort ocr.res"

fetch_one() {
    local name="$1" dest="$MODEL_DIR/$1"
    if [ -f "$dest" ] && [ "$(wc -c < "$dest")" -gt 1024 ]; then
        info "$name already present, skipping."
        return
    fi
    info "Fetching $name ..."
    if command -v curl &>/dev/null; then
        curl -fSL --retry 3 -o "$dest.part" "$HF_BASE/$name"
    elif command -v wget &>/dev/null; then
        wget -q --tries=3 -O "$dest.part" "$HF_BASE/$name"
    else
        error "Neither curl nor wget found."
        exit 1
    fi
    if [ ! -s "$dest.part" ] || [ "$(wc -c < "$dest.part")" -le 1024 ]; then
        error "$name download looks truncated/empty - aborting."
        rm -f "$dest.part"
        exit 1
    fi
    mv "$dest.part" "$dest"
    success "$name fetched."
}

for f in $FILES; do
    fetch_one "$f"
done

success "All deepdoc models present."
echo ""
echo "  Model directory: $MODEL_DIR"
echo ""
echo "  Use it from Go (opt-in - see pkg/nativedoc's own doc comment):"
echo "    conv := nativedoc.New(\"$MODEL_DIR\")"
echo "    cfg := &sdk.ClientConfig{DocumentConverter: conv}"
echo ""
