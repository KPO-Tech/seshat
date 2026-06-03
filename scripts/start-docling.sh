#!/usr/bin/env bash
# start-docling.sh — launch a local docling-serve instance for document conversion.
#
# Usage:
#   ./scripts/start-docling.sh                     # port 5001, CPU-only, 1 worker
#   DOCLING_PORT=5002 ./scripts/start-docling.sh
#   DOCLING_WORKERS=4 ./scripts/start-docling.sh   # 4 parallel workers (GPU machine)
#
# Environment variables:
#   DOCLING_HOST     bind address (default: 127.0.0.1)
#   DOCLING_PORT     HTTP port    (default: 5001)
#   DOCLING_WORKERS  parallel conversion workers (default: 1)
#
# Nexus Engine integration:
#   Set NEXUS_DOCLING_URL=http://localhost:5001 (or DOCLING_URL in config YAML).
#   When set, nexus-engine enables:
#     - read_file:          PDF, DOCX, PPTX, XLSX, audio transcription, image OCR
#     - read_document_url:  remote document fetch + conversion
#     - session uploads:    auto-convert on upload, store markdown_path
#     - RAG ingest:         binary documents chunked as markdown
#
# MCP bridge (optional):
#   To expose docling as an MCP server so the agent can call it directly, add
#   the following to ~/.nexus/mcp.json (or the project .mcp.json):
#
#     {
#       "mcpServers": {
#         "docling": {
#           "type": "stdio",
#           "command": "python3",
#           "args": ["/absolute/path/to/scripts/docling-mcp-bridge.py"],
#           "env": { "DOCLING_URL": "http://localhost:5001" }
#         }
#       }
#     }
#
#   Requirements for the MCP bridge: pip install requests
#
# Requirements:
#   pip install docling-serve   (or: pip install "docling[serve]")
#
# docling-serve GitHub: https://github.com/docling-project/docling-serve

set -euo pipefail

PORT="${DOCLING_PORT:-5001}"
HOST="${DOCLING_HOST:-127.0.0.1}"
WORKERS="${DOCLING_WORKERS:-1}"

command -v docling-serve &>/dev/null || {
  echo "[docling] docling-serve not found. Installing..."
  pip install --quiet "docling[serve]"
}

echo "[docling] Starting docling-serve on ${HOST}:${PORT} (workers: ${WORKERS})"
echo "[docling] Set NEXUS_DOCLING_URL=http://${HOST}:${PORT} in your nexus config."
echo ""

exec docling-serve run \
  --host "${HOST}" \
  --port "${PORT}" \
  --workers "${WORKERS}"
