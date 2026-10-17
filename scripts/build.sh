#!/bin/bash
# build.sh — Build script for Seshat

set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

echo -e "${GREEN}=== Seshat Build Script ===${NC}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

echo "Project root: $PROJECT_ROOT"

echo -e "${YELLOW}Cleaning previous builds...${NC}"
rm -rf bin/

echo -e "${YELLOW}Building CLI binary...${NC}"
mkdir -p bin
go build -o bin/seshat ./tui/cmd/cli

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Build successful!${NC}"
    echo "Binary: $PROJECT_ROOT/bin/seshat"
    BINARY_SIZE=$(ls -lh bin/seshat | awk '{print $5}')
    echo "Size: $BINARY_SIZE"
    GO_VERSION=$(go version | awk '{print $3}')
    echo "Go: $GO_VERSION"
    exit 0
else
    echo -e "${RED}✗ Build failed!${NC}"
    exit 1
fi
