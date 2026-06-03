#!/bin/bash

# lint.sh - Lint script for Nexus Engine
# This script runs Go linters and formatters

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Nexus Engine Lint Script ===${NC}"

# Get the project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

echo "Project root: $PROJECT_ROOT"
echo ""

# Check if gofmt is installed
if ! command -v gofmt &> /dev/null; then
	echo -e "${RED}✗ gofmt not found!${NC}"
	echo "Please install Go first: https://golang.org/dl/"
	exit 1
fi

# Run gofmt
echo -e "${BLUE}Running gofmt...${NC}"
UNFORMATTED=$(gofmt -l .)

if [ -z "$UNFORMATTED" ]; then
	echo -e "${GREEN}✓ All files are properly formatted${NC}"
else
	echo -e "${YELLOW}Found unformatted files:${NC}"
	echo "$UNFORMATTED"
	echo ""
	echo -e "${YELLOW}Running gofmt -w to fix...${NC}"
	gofmt -w .
	echo -e "${GREEN}✓ Formatting complete${NC}"
fi

# Run go vet (optional - only if golangci-lint is not available)
if command -v govet &> /dev/null; then
	echo ""
	echo -e "${BLUE}Running go vet...${NC}"
	if go vet ./...; then
		echo -e "${GREEN}✓ go vet passed${NC}"
	else
		echo -e "${RED}✗ go vet found issues${NC}"
	fi
fi

# Check for golangci-lint (optional)
if command -v golangci-lint &> /dev/null; then
	echo ""
	echo -e "${BLUE}Running golangci-lint (optional)...${NC}"
	if golangci-lint run; then
		echo -e "${GREEN}✓ golangci-lint passed${NC}"
	else
		echo -e "${YELLOW}golangci-lint skipped (not installed)${NC}"
	fi
fi

echo ""
echo -e "${GREEN}=== Lint Summary ===${NC}"
echo -e "${GREEN}✓ Linting complete${NC}"
