BINARY   := nexus-engine
CMD_API  := ./cmd/api
CMD_CLI  := ./cmd/cli

.PHONY: all build build-api build-cli test test-race lint clean tidy

all: build

build: build-api build-cli

build-api:
	go build -o bin/$(BINARY)-api $(CMD_API)

build-cli:
	go build -o bin/$(BINARY)-cli $(CMD_CLI)

test:
	go test ./... -timeout 300s

test-race:
	go test -race ./... -timeout 300s

test-db:
	go test -race ./internal/db/... -v -timeout 120s

lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint not installed — run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/
