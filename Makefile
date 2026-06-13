CMD_CLI  := ./cmd/cli
CMD_GRPC := ./cmd/grpc

.PHONY: all build build-cli build-grpc test test-race lint fmt vet tidy clean hooks install-deps

all: build

build: build-cli build-grpc

build-cli:
	go build -o bin/nexus $(CMD_CLI)

build-grpc:
	go build -o bin/nexus-grpc $(CMD_GRPC)

test:
	go test ./... -timeout 300s

test-race:
	go test -race ./... -timeout 300s

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint not installed — run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/

# Install git hooks from .githooks/ (run once after cloning).
hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"

# Install external runtime dependencies (ripgrep).
install-deps:
	@echo "Checking for ripgrep (rg)..."
	@if command -v rg > /dev/null 2>&1; then \
		echo "ripgrep already installed: $$(rg --version | head -1)"; \
	elif [ "$$(uname)" = "Darwin" ]; then \
		brew install ripgrep; \
	elif [ -f /etc/debian_version ]; then \
		sudo apt-get update -qq && sudo apt-get install -y ripgrep; \
	elif [ -f /etc/arch-release ]; then \
		sudo pacman -S --noconfirm ripgrep; \
	elif [ -f /etc/fedora-release ] || [ -f /etc/redhat-release ]; then \
		sudo dnf install -y ripgrep; \
	else \
		echo "Could not detect package manager. Install ripgrep manually: https://github.com/BurntSushi/ripgrep#installation" && exit 1; \
	fi
