#!/bin/bash

# test.sh - Test script for Nexus Engine
# This script runs all tests in the project

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Nexus Engine Test Script ===${NC}"

# Get the project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

echo "Project root: $PROJECT_ROOT"
echo ""

# Run tests for each package
PACKAGES=(
	"internal/execution"
	"internal/tools/contract"
	"internal/types"
	"pkg/sdk"
)

FAILED_TESTS=()
PASSED_TESTS=()

echo -e "${BLUE}Running tests...${NC}"
echo ""

for pkg in "${PACKAGES[@]}"; do
	echo -e "${YELLOW}Testing: $pkg${NC}"

	if go test "./$pkg" -v 2>&1; then
		echo -e "${GREEN}✓ PASSED${NC}"
		PASSED_TESTS+=("$pkg")
	else
		echo -e "${RED}✗ FAILED${NC}"
		FAILED_TESTS+=("$pkg")
	fi

	echo ""
done

# Summary
echo -e "${GREEN}=== Test Summary ===${NC}"
echo -e "${GREEN}Passed: ${#PASSED_TESTS[@]}${NC}"
echo -e "${RED}Failed: ${#FAILED_TESTS[@]}${NC}"

if [ ${#FAILED_TESTS[@]} -eq 0 ]; then
	echo -e "${GREEN}✓ All tests passed!${NC}"
	exit 0
else
	echo -e "${RED}✗ Some tests failed!${NC}"
	exit 1
fi
