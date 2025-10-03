#!/bin/bash
# Entity Management Tests - Pre-compiled Runner
# This script compiles tests first to avoid go test hanging issues

set -e

BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

TEST_BINARY="./entity_tests.exe"

echo -e "${BOLD}Entity Management Tests - Pre-compiled Runner${NC}\n"

# Compile tests
echo -e "${YELLOW}Compiling tests...${NC}"
go test -c -o "$TEST_BINARY" ./tests/entityManagement
echo -e "${GREEN}✓ Tests compiled successfully${NC}\n"

# Run tests based on argument
case "${1:-all}" in
  all)
    echo -e "${GREEN}Running all tests...${NC}"
    $TEST_BINARY -test.v -test.timeout=30s
    ;;

  integration)
    echo -e "${GREEN}Running integration tests...${NC}"
    $TEST_BINARY -test.v -test.run TestIntegration -test.timeout=30s
    ;;

  security)
    echo -e "${GREEN}Running security tests...${NC}"
    $TEST_BINARY -test.v -test.run TestSecurity -test.timeout=30s
    ;;

  hooks)
    echo -e "${GREEN}Running hook system tests...${NC}"
    $TEST_BINARY -test.v -test.run TestHooks -test.timeout=30s
    ;;

  relationships)
    echo -e "${GREEN}Running relationship filter tests...${NC}"
    $TEST_BINARY -test.v -test.run TestRelationships -test.timeout=30s
    ;;

  generic)
    echo -e "${GREEN}Running generic service/repository tests...${NC}"
    $TEST_BINARY -test.v -test.run "TestGeneric" -test.timeout=30s
    ;;

  registration)
    echo -e "${GREEN}Running entity registration tests...${NC}"
    $TEST_BINARY -test.v -test.run "TestEntityRegistration" -test.timeout=30s
    ;;

  bench)
    echo -e "${GREEN}Running benchmarks...${NC}"
    $TEST_BINARY -test.bench=. -test.benchmem -test.run=^$
    ;;

  race)
    echo -e "${YELLOW}Running tests with race detector...${NC}"
    echo -e "${YELLOW}Recompiling with race detector...${NC}"
    go test -c -race -o "$TEST_BINARY" ./tests/entityManagement
    $TEST_BINARY -test.v -test.timeout=60s
    ;;

  cover)
    echo -e "${GREEN}Generating coverage report...${NC}"
    go test -v -coverprofile=coverage.out -covermode=atomic ./tests/entityManagement
    go tool cover -html=coverage.out -o coverage.html
    echo -e "${GREEN}✓ Coverage report: coverage.html${NC}"
    ;;

  quick)
    echo -e "${YELLOW}Running quick validation...${NC}"
    $TEST_BINARY -test.v -test.short -test.timeout=10s
    ;;

  clean)
    echo -e "${YELLOW}Cleaning up...${NC}"
    rm -f "$TEST_BINARY" coverage.out coverage.html
    echo -e "${GREEN}✓ Cleaned up test artifacts${NC}"
    ;;

  help)
    echo "Usage: ./run_tests_compiled.sh [option]"
    echo ""
    echo "Options:"
    echo "  all            - Run all tests (default)"
    echo "  integration    - Run integration tests only"
    echo "  security       - Run security/permission tests only"
    echo "  hooks          - Run hook system tests only"
    echo "  relationships  - Run relationship filter tests only"
    echo "  generic        - Run generic service/repository tests"
    echo "  registration   - Run entity registration tests"
    echo "  bench          - Run all benchmarks"
    echo "  race           - Run with race detector"
    echo "  cover          - Generate HTML coverage report"
    echo "  quick          - Quick validation"
    echo "  clean          - Clean up test artifacts"
    echo "  help           - Show this help"
    echo ""
    echo "Examples:"
    echo "  ./run_tests_compiled.sh                # Run all tests"
    echo "  ./run_tests_compiled.sh integration    # Integration tests only"
    echo "  ./run_tests_compiled.sh race           # Run with race detector"
    ;;

  *)
    echo -e "${RED}Unknown option: $1${NC}"
    echo "Run './run_tests_compiled.sh help' for usage"
    exit 1
    ;;
esac

echo -e "\n${GREEN}✓ Done!${NC}"
