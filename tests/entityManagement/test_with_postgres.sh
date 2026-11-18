#!/bin/bash
# Quick test script with PostgreSQL
# Starts PostgreSQL, runs tests, and cleans up

set -e

BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BOLD}OCF Core - PostgreSQL Test Runner${NC}\n"

# Check if docker is available
if ! command -v docker &> /dev/null; then
    echo -e "${RED}❌ Docker not found. Please install Docker first.${NC}"
    exit 1
fi

# Start PostgreSQL with docker compose
echo -e "${YELLOW}🐘 Starting PostgreSQL container...${NC}"
docker compose -f ../../docker-compose.test.yml up -d postgres-test

# Wait for PostgreSQL to be ready
echo -e "${YELLOW}⏳ Waiting for PostgreSQL to be ready...${NC}"
timeout 30s bash -c 'until docker exec ocf-postgres-test pg_isready -U postgres &>/dev/null; do sleep 1; done' || {
    echo -e "${RED}❌ PostgreSQL failed to start in time${NC}"
    docker compose -f ../../docker-compose.test.yml logs postgres-test
    exit 1
}

echo -e "${GREEN}✅ PostgreSQL is ready!${NC}\n"

# Set environment variables
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=postgres
export POSTGRES_PASSWORD=postgres
export POSTGRES_DB=ocf_test
export POSTGRES_SSLMODE=disable

# Run tests
echo -e "${BOLD}🧪 Running tests...${NC}\n"

if [ "$1" == "quick" ]; then
    echo -e "${YELLOW}Running quick SQLite tests only...${NC}"
    ./run_tests_compiled.sh quick
elif [ "$1" == "all" ] || [ -z "$1" ]; then
    echo -e "${YELLOW}Running full test suite (SQLite + PostgreSQL)...${NC}"
    ./run_tests_compiled.sh all
else
    echo -e "${RED}Unknown option: $1${NC}"
    echo "Usage: $0 [quick|all]"
    exit 1
fi

# Cleanup option
if [ "$2" == "cleanup" ] || [ "$CLEANUP" == "true" ]; then
    echo -e "\n${YELLOW}🧹 Cleaning up PostgreSQL container...${NC}"
    docker compose -f ../../docker-compose.test.yml down -v
    echo -e "${GREEN}✅ Cleanup complete${NC}"
else
    echo -e "\n${YELLOW}💡 PostgreSQL container is still running${NC}"
    echo -e "   To stop it: ${BOLD}docker compose -f docker-compose.test.yml down${NC}"
    echo -e "   To clean up: ${BOLD}CLEANUP=true $0${NC}"
fi

echo -e "\n${GREEN}✅ Tests completed!${NC}"
