#!/usr/bin/env bash
# start-docling.sh — launch docling-serve from the Nexus-managed Python venv.
#
# Usage:
#   ./scripts/start-docling.sh
#   DOCLING_PORT=5002 ./scripts/start-docling.sh
#   NEXUS_RUNTIME_ROOT=/custom/path ./scripts/start-docling.sh
#
# Environment variables:
#   NEXUS_RUNTIME_ROOT   Config/data root (default: ~/.config/nexus-cli)
#   DOCLING_HOST         Bind address (default: 127.0.0.1)
#   DOCLING_PORT         HTTP port (default: 5001)
#   DOCLING_WORKERS      Parallel conversion workers (default: 1)
#
# Nexus auto-starts docling-serve at launch when the venv is installed.
# Use this script only if you want to run it manually (e.g. as a daemon).

set -euo pipefail

_default_runtime_root() {
    if [ -n "${XDG_CONFIG_HOME:-}" ]; then
        echo "$XDG_CONFIG_HOME/nexus-cli"
    else
        echo "$HOME/.config/nexus-cli"
    fi
}

NEXUS_RUNTIME_ROOT="${NEXUS_RUNTIME_ROOT:-$(_default_runtime_root)}"
VENV_DIR="$NEXUS_RUNTIME_ROOT/.venv"
DOCLING_BIN="$VENV_DIR/bin/docling-serve"

PORT="${DOCLING_PORT:-5001}"
HOST="${DOCLING_HOST:-127.0.0.1}"
WORKERS="${DOCLING_WORKERS:-1}"

# ── Ensure venv + docling-serve are installed ─────────────────────────────────
if [ ! -x "$DOCLING_BIN" ]; then
    echo "[docling] docling-serve not found. Running: ./scripts/install-python-env.sh"
    "$(dirname "$0")/install-python-env.sh"
fi

# ── Launch ────────────────────────────────────────────────────────────────────
echo "[docling] Starting docling-serve on ${HOST}:${PORT} (workers: ${WORKERS})"
echo "[docling] Logs: $NEXUS_RUNTIME_ROOT/logs/docling.log"
echo ""

exec "$DOCLING_BIN" run \
  --host "${HOST}" \
  --port "${PORT}" \
  --workers "${WORKERS}"
