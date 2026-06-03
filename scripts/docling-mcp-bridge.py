#!/usr/bin/env python3
"""
docling-mcp-bridge.py — a stdio MCP server that wraps a running docling-serve instance.

Wire it in .mcp.json (or the global ~/.nexus/mcp.json):

  {
    "mcpServers": {
      "docling": {
        "type": "stdio",
        "command": "python3",
        "args": ["/absolute/path/to/scripts/docling-mcp-bridge.py"],
        "env": { "DOCLING_URL": "http://localhost:5001" }
      }
    }
  }

Requirements:
  pip install requests

The bridge exposes two tools:
  - convert_document_file  Convert a local file (PDF, DOCX, PPTX, XLSX, WAV, MP3, images)
  - convert_document_url   Fetch and convert a remote document (arXiv, DOCX URLs, HTML pages)
"""

import json
import os
import sys
from typing import Any

try:
    import requests
except ImportError:
    sys.stderr.write("[docling-mcp-bridge] requests not installed. Run: pip install requests\n")
    sys.exit(1)

DOCLING_URL = os.environ.get("DOCLING_URL", "http://localhost:5001").rstrip("/")
CONVERT_ENDPOINT = f"{DOCLING_URL}/v1alpha/convert/source"
REQUEST_TIMEOUT = int(os.environ.get("DOCLING_TIMEOUT", "120"))

TOOLS = [
    {
        "name": "convert_document_file",
        "description": (
            "Convert a local document to markdown using docling-serve. "
            "Supports: PDF, DOCX, PPTX, XLSX, WAV (transcription), MP3 (transcription), "
            "PNG, JPEG, TIFF (OCR), HTML, LaTeX. "
            "Returns the full extracted markdown text."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "file_path": {
                    "type": "string",
                    "description": "Absolute path to the local document to convert.",
                },
                "include_images": {
                    "type": "boolean",
                    "description": "When true, include base64-encoded extracted images in the response. Default false.",
                },
            },
            "required": ["file_path"],
        },
    },
    {
        "name": "convert_document_url",
        "description": (
            "Fetch and convert a remote document to markdown using docling-serve. "
            "Works with arXiv PDFs, hosted DOCX/PPTX files, and any HTML page. "
            "Returns the full extracted markdown text."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "url": {
                    "type": "string",
                    "description": "HTTP/HTTPS URL of the document to fetch and convert.",
                },
                "include_images": {
                    "type": "boolean",
                    "description": "When true, include base64-encoded extracted images in the response. Default false.",
                },
            },
            "required": ["url"],
        },
    },
]


# ── docling-serve calls ────────────────────────────────────────────────────────

def _parse_response(body: dict, include_images: bool) -> str:
    status = body.get("status", "")
    if status not in ("success", "partial_success"):
        errors = "; ".join(body.get("errors") or ["unknown error"])
        raise ValueError(f"docling conversion failed ({status}): {errors}")
    doc = body.get("document")
    if not doc:
        raise ValueError("docling returned no document")

    parts = [doc.get("md_content", "")]

    if include_images:
        for i, pic in enumerate(doc.get("pictures") or []):
            img = pic.get("image")
            if not img:
                continue
            uri = img.get("uri", "")
            mime = img.get("mimetype", "image/png")
            if uri.startswith("data:"):
                parts.append(f"\n\n<!-- image {i + 1} ({mime}) -->\n![]({uri})")

    return "\n".join(parts)


def call_convert_file(file_path: str, include_images: bool) -> str:
    with open(file_path, "rb") as fh:
        data = fh.read()
    filename = os.path.basename(file_path)
    resp = requests.post(
        CONVERT_ENDPOINT,
        files={"file": (filename, data)},
        headers={"Accept": "application/json"},
        timeout=REQUEST_TIMEOUT,
    )
    resp.raise_for_status()
    return _parse_response(resp.json(), include_images)


def call_convert_url(url: str, include_images: bool) -> str:
    resp = requests.post(
        CONVERT_ENDPOINT,
        json={"http_source": {"url": url}},
        headers={"Accept": "application/json"},
        timeout=REQUEST_TIMEOUT,
    )
    resp.raise_for_status()
    return _parse_response(resp.json(), include_images)


# ── MCP JSON-RPC helpers ───────────────────────────────────────────────────────

def _write(msg: dict) -> None:
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def _ok(msg_id: Any, result: Any) -> None:
    _write({"jsonrpc": "2.0", "id": msg_id, "result": result})


def _err(msg_id: Any, message: str, code: int = -32000) -> None:
    _write({"jsonrpc": "2.0", "id": msg_id, "error": {"code": code, "message": message}})


def _tool_result(msg_id: Any, text: str, is_error: bool = False) -> None:
    _ok(msg_id, {
        "content": [{"type": "text", "text": text}],
        "isError": is_error,
    })


# ── request dispatcher ────────────────────────────────────────────────────────

def handle(req: dict) -> None:
    method = req.get("method", "")
    msg_id = req.get("id")          # None for notifications
    params = req.get("params") or {}

    if method == "initialize":
        _ok(msg_id, {
            "protocolVersion": "2024-11-05",
            "serverInfo": {"name": "docling-mcp-bridge", "version": "1.0.0"},
            "capabilities": {"tools": {}},
        })

    elif method == "initialized":
        pass  # notification — no response

    elif method == "ping":
        _ok(msg_id, {})

    elif method == "tools/list":
        _ok(msg_id, {"tools": TOOLS})

    elif method == "tools/call":
        name = params.get("name")
        args = params.get("arguments") or {}
        include_images = bool(args.get("include_images", False))
        try:
            if name == "convert_document_file":
                file_path = args.get("file_path", "")
                if not file_path:
                    _tool_result(msg_id, "file_path is required", is_error=True)
                    return
                if not os.path.isabs(file_path):
                    _tool_result(msg_id, f"file_path must be an absolute path: {file_path}", is_error=True)
                    return
                markdown = call_convert_file(file_path, include_images)
                _tool_result(msg_id, markdown)

            elif name == "convert_document_url":
                url = args.get("url", "")
                if not url:
                    _tool_result(msg_id, "url is required", is_error=True)
                    return
                if not url.startswith(("http://", "https://")):
                    _tool_result(msg_id, "url must start with http:// or https://", is_error=True)
                    return
                markdown = call_convert_url(url, include_images)
                _tool_result(msg_id, markdown)

            else:
                _tool_result(msg_id, f"unknown tool: {name}", is_error=True)

        except FileNotFoundError:
            _tool_result(msg_id, f"file not found: {args.get('file_path')}", is_error=True)
        except requests.exceptions.ConnectionError:
            _tool_result(msg_id, f"cannot connect to docling-serve at {DOCLING_URL} — is it running?", is_error=True)
        except requests.exceptions.Timeout:
            _tool_result(msg_id, "docling-serve timed out — try a smaller file or increase DOCLING_TIMEOUT", is_error=True)
        except Exception as exc:
            _tool_result(msg_id, f"error: {exc}", is_error=True)

    elif msg_id is not None:
        _err(msg_id, f"method not found: {method}", code=-32601)


# ── main loop ─────────────────────────────────────────────────────────────────

def main() -> None:
    sys.stderr.write(f"[docling-mcp-bridge] starting — docling-serve at {DOCLING_URL}\n")
    for raw_line in sys.stdin:
        line = raw_line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError as exc:
            sys.stderr.write(f"[docling-mcp-bridge] JSON parse error: {exc}\n")
            continue
        try:
            handle(req)
        except Exception as exc:
            sys.stderr.write(f"[docling-mcp-bridge] unhandled error in {req.get('method')}: {exc}\n")


if __name__ == "__main__":
    main()
