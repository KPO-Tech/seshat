CMD_CLI        := ./cmd/cli
CMD_GRPC       := ./cmd/grpc
CMD_SLACK_BOT  := ./cmd/slack-bot
CMD_AUTOMATION := ./cmd/automation

# Make built binaries discoverable from the repo root.
export PATH := $(CURDIR)/bin:$(PATH)

.PHONY: all build build-cli build-grpc build-slack-bot build-automation build_linux test test-race fmt vet lint tidy \
        clean clean-runtime clean-all hooks setup install-python start-docling slack-bot install-deepdoc-models

# ── Default ────────────────────────────────────────────────────────────────────

all: build

# ── First-time setup ──────────────────────────────────────────────────────────
# Installs all dependencies (ripgrep, uv, Python venv + docling-serve),
# builds the binaries, and wires git hooks.
#
# Linux / macOS:
#   make setup
#
# Windows (PowerShell — make is not available by default):
#   powershell -ExecutionPolicy Bypass -File scripts\setup.ps1

setup:
	@case "$$(uname -s 2>/dev/null)" in \
		Darwin|Linux) bash scripts/setup.sh ;; \
		*) \
			echo "" ; \
			echo "  Windows detected — make is not available by default." ; \
			echo "  Open PowerShell and run:" ; \
			echo "    powershell -ExecutionPolicy Bypass -File scripts\\setup.ps1" ; \
			echo "" ;; \
	esac

# ── Build ──────────────────────────────────────────────────────────────────────

build: build-cli build-grpc build-slack-bot build-automation

build-cli:
	go build -o bin/seshat $(CMD_CLI)


build-grpc:
	go build -o bin/seshat-grpc $(CMD_GRPC)

build-slack-bot:
	go build -o bin/seshat-slack $(CMD_SLACK_BOT)

build-automation:
	go build -o bin/seshat-auto $(CMD_AUTOMATION)

slack-bot:
	@export $$(grep -v '^#' private/.env.slack | xargs) && \
	go run $(CMD_SLACK_BOT)

build_linux:
	go build -o /tmp/seshat $(CMD_CLI)

# ── Test ───────────────────────────────────────────────────────────────────────

test:
	go test ./... -timeout 300s

test-race:
	go test -race ./... -timeout 300s

# ── Code quality ───────────────────────────────────────────────────────────────

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	@which golangci-lint > /dev/null 2>&1 \
		|| (echo "golangci-lint not installed — run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

tidy:
	go mod tidy

# ── Maintenance ────────────────────────────────────────────────────────────────

# Remove compiled binaries. Safe to run anytime.
clean:
	rm -rf bin/

# Erase all seshat runtime data (DB, credentials, sessions, logs, venv).
# WARNING: credentials and session history cannot be recovered — you will need
# to re-run `seshat login` and `seshat config` afterwards.
# Uses SESHAT_RUNTIME_ROOT if set, otherwise falls back to ~/.config/seshat-*.
clean-runtime:
	@confdir="$${SESHAT_RUNTIME_ROOT:-$${XDG_CONFIG_HOME:-$$HOME/.config}}" ; \
	for d in seshat-cli seshat-tui seshat-slack ; do \
	    target="$$confdir/$$d" ; \
	    if [ -d "$$target" ]; then \
	        rm -rf "$$target" && echo "  removed $$target" ; \
	    fi ; \
	done
	@echo "Runtime data cleared."

# Full wipe: binaries + all runtime data. Useful for a completely fresh start.
clean-all: clean clean-runtime

# (Re-)install git pre-commit hooks from .githooks/.
hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"

# ── Python / docling (optional feature) ───────────────────────────────────────
# install-python creates the managed venv and installs docling-serve.
# It is called automatically by `make setup`; use it to update or reinstall.
#
# Options (env vars):
#   DOCLING_EXTRAS=gpu      → GPU-accelerated conversion
#   PYTHON_VERSION=3.12     → specific Python version

install-python:
	@./scripts/install-python-env.sh

# Start docling-serve manually.
# Seshat auto-starts it at launch when the venv is installed — this is only
# needed if you want to run it as a standalone process.

start-docling:
	@./scripts/start-docling.sh

# ── pkg/nativedoc (optional, opt-in) ──────────────────────────────────────────
# Fully native PDF/DOCX/XLSX conversion (including OCR/layout for scanned
# PDFs) with no external service - not wired in anywhere by default, an
# embedder opts in via sdk.ClientConfig.DocumentConverter. See
# pkg/nativedoc's own doc comment and docs/issues/document-intelligence-roadmap.md.
#
# install-deepdoc-models fetches the ONNX models. Building anything that
# imports pkg/nativedoc also needs native libs (pdf_oxide/pdfium/onnxruntime)
# and the `static` build tag - that's scripts/setup-nativedoc-cgo.sh, which
# prints shell exports rather than being a Make target (Make can't export
# into your calling shell):
#   source <(./scripts/setup-nativedoc-cgo.sh)
#   go build -tags "static nativedoc" ./your/package/...

install-deepdoc-models:
	@./scripts/install-deepdoc-models.sh
