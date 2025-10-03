#!/bin/bash
# Entity Management System - Test Runner
# Usage: ./RUN_TESTS.sh [option]

set -e

BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BOLD}Entity Management System - Test Suite${NC}\n"

case "${1:-all}" in
  all)
    echo -e "${GREEN}Running all tests...${NC}"
    go test ./tests/entityManagement/ -v
    ;;

  integration)
    echo -e "${GREEN}Running integration tests...${NC}"
    go test ./tests/entityManagement/ -run TestIntegration -v
    ;;

  security)
    echo -e "${GREEN}Running security tests...${NC}"
    go test ./tests/entityManagement/ -run TestSecurity -v
    ;;

  hooks)
    echo -e "${GREEN}Running hook system tests...${NC}"
    go test ./tests/entityManagement/ -run TestHooks -v
    ;;

  relationships)
    echo -e "${GREEN}Running relationship filter tests...${NC}"
    go test ./tests/entityManagement/ -run TestRelationships -v
    ;;

  bench)
    echo -e "${GREEN}Running benchmarks...${NC}"
    go test ./tests/entityManagement/ -bench=. -benchmem -run=^$
    ;;

  bench-baseline)
    echo -e "${YELLOW}Saving baseline benchmarks to benchmarks_baseline.txt...${NC}"
    go test ./tests/entityManagement/ -bench=. -benchmem -run=^$ > benchmarks_baseline.txt
    echo -e "${GREEN}✓ Baseline saved!${NC}"
    echo "Compare later with: benchstat benchmarks_baseline.txt benchmarks_new.txt"
    ;;

  race)
    echo -e "${YELLOW}Running tests with race detector...${NC}"
    go test ./tests/entityManagement/ -race -v
    ;;

  cover)
    echo -e "${GREEN}Generating coverage report...${NC}"
    go test ./tests/entityManagement/ -v -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    echo -e "${GREEN}✓ Coverage report: coverage.html${NC}"
    ;;

  quick)
    echo -e "${YELLOW}Running quick validation (no benchmarks)...${NC}"
    go test ./tests/entityManagement/ -v -short
    ;;

  help)
    echo "Usage: ./RUN_TESTS.sh [option]"
    echo ""
    echo "Options:"
    echo "  all            - Run all tests (default)"
    echo "  integration    - Run integration tests only"
    echo "  security       - Run security/permission tests only"
    echo "  hooks          - Run hook system tests only"
    echo "  relationships  - Run relationship filter tests only"
    echo "  bench          - Run all benchmarks"
    echo "  bench-baseline - Save benchmark baseline for comparison"
    echo "  race           - Run with race detector"
    echo "  cover          - Generate HTML coverage report"
    echo "  quick          - Quick validation (skip slow tests)"
    echo "  help           - Show this help"
    echo ""
    echo "Examples:"
    echo "  ./RUN_TESTS.sh                    # Run all tests"
    echo "  ./RUN_TESTS.sh integration        # Integration tests only"
    echo "  ./RUN_TESTS.sh bench-baseline     # Before refactoring"
    echo "  ./RUN_TESTS.sh bench > after.txt  # After refactoring"
    ;;

  *)
    echo -e "${RED}Unknown option: $1${NC}"
    echo "Run './RUN_TESTS.sh help' for usage"
    exit 1
    ;;
esac

echo -e "\n${GREEN}✓ Done!${NC}"
