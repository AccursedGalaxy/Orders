#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo "Running pre-commit hooks..."

# Get staged Go files
STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep "\.go$")

# Exit if no Go files are staged
if [[ "$STAGED_GO_FILES" = "" ]]; then
    echo -e "${GREEN}No Go files staged, skipping pre-commit hooks${NC}"
    exit 0
fi

# Format code
echo "Formatting code..."
make fmt

# Run linter
echo "Running linter..."
make lint
if [ $? -ne 0 ]; then
    echo -e "${RED}Linting failed${NC}"
    exit 1
fi

# Run tests
echo "Running tests..."
make test
if [ $? -ne 0 ]; then
    echo -e "${RED}Tests failed${NC}"
    exit 1
fi

# Check if any files were formatted
UNSTAGED_CHANGES=$(git diff --name-only | grep "\.go$")
if [[ "$UNSTAGED_CHANGES" != "" ]]; then
    echo -e "${RED}Some files were formatted. Please stage the changes:${NC}"
    echo "$UNSTAGED_CHANGES"
    exit 1
fi

echo -e "${GREEN}All pre-commit hooks passed!${NC}"
exit 0 