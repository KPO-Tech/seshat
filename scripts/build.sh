#!/bin/bash

# build.sh - Build script for Nexus Engine
# This script compiles the project and creates the main binary

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Nexus Engine Build Script ===${NC}"

# Get the project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

echo "Project root: $PROJECT_ROOT"

# Clean previous builds
echo -e "${YELLOW}Cleaning previous builds...${NC}"
rm -f nexus
rm -rf bin/

# Build the main CLI binary
echo -e "${YELLOW}Building CLI binary...${NC}"
go build -o nexus ./cmd/nexus

if [ $? -eq 0 ]; then
	echo -e "${GREEN}✓ Build successful!${NC}"
	echo "Binary created: $PROJECT_ROOT/nexus"

	# Show binary size
	BINARY_SIZE=$(ls -lh nexus | awk '{print $5}')
	echo "Binary size: $BINARY_SIZE"

	# Show Go version
	GO_VERSION=$(go version | awk '{print $3}')
	echo "Go version: $GO_VERSION"

	exit 0
else
	echo -e "${RED}✗ Build failed!${NC}"
	exit 1
fi
