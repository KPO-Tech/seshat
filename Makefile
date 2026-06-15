CMD_CLI  := ./cmd/cli
CMD_GRPC := ./cmd/grpc

.PHONY: all build build-cli build-grpc test test-race fmt vet lint tidy clean hooks \
        setup install-python start-docling

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

build: build-cli build-grpc

build-cli:
	go build -o bin/nexus $(CMD_CLI)

build-grpc:
	go build -o bin/nexus-grpc $(CMD_GRPC)

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

clean:
	rm -rf bin/

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
# Nexus auto-starts it at launch when the venv is installed — this is only
# needed if you want to run it as a standalone process.

start-docling:
	@./scripts/start-docling.sh
