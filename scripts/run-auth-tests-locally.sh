#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}🧪 Running Auth Tests Locally${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# Check if JWT key exists
if [ ! -f "./src/auth/casdoor/token_jwt_key.pem" ]; then
    echo -e "${RED}❌ Error: JWT key not found at ./src/auth/casdoor/token_jwt_key.pem${NC}"
    echo -e "${YELLOW}💡 You need to place the test JWT key in this location${NC}"
    exit 1
fi

# Clean up any previous test containers
echo -e "${YELLOW}🧹 Cleaning up old test containers...${NC}"
docker-compose -f docker-compose.test.yml down -v --remove-orphans 2>/dev/null || true
echo ""

# Start database services first
echo -e "${BLUE}🐳 Starting database services (PostgreSQL + MySQL)...${NC}"
docker-compose -f docker-compose.test.yml up -d postgres-test casdoor-db-test

echo -e "${YELLOW}⏳ Waiting 15 seconds for databases to initialize...${NC}"
sleep 15

# Start Casdoor
echo -e "${BLUE}🐳 Starting Casdoor service...${NC}"
docker-compose -f docker-compose.test.yml up -d casdoor-test

echo -e "${YELLOW}⏳ Waiting 10 seconds for Casdoor to start...${NC}"
sleep 10
echo ""

# Check services health
echo -e "${BLUE}🔍 Checking service health...${NC}"
echo -e "PostgreSQL: $(docker inspect ocf-postgres-test --format='{{.State.Health.Status}}' 2>/dev/null || echo 'not running')"
echo -e "MySQL: $(docker inspect ocf-casdoor-db-test --format='{{.State.Health.Status}}' 2>/dev/null || echo 'not running')"
echo -e "Casdoor: $(docker inspect ocf-casdoor-test --format='{{.State.Health.Status}}' 2>/dev/null || echo 'not running')"
echo ""

# Show container status
echo -e "${BLUE}📊 Container Status:${NC}"
docker-compose -f docker-compose.test.yml ps
echo ""

# Set environment variables
export POSTGRES_DB="ocf_test"
export POSTGRES_USER="postgres"
export POSTGRES_PASSWORD="postgres"
export POSTGRES_HOST="ocf-postgres-test"
export POSTGRES_PORT="5432"
export POSTGRES_SSLMODE="disable"
export CASDOOR_ENDPOINT="http://ocf-casdoor-test:8000"

# Run tests
echo -e "${GREEN}🔐 Running auth tests...${NC}"
echo ""

docker run --rm \
    --network ocf-core_test-network \
    -v $(pwd):/workspace \
    -w /workspace \
    -e POSTGRES_DB=$POSTGRES_DB \
    -e POSTGRES_USER=$POSTGRES_USER \
    -e POSTGRES_PASSWORD=$POSTGRES_PASSWORD \
    -e POSTGRES_HOST=$POSTGRES_HOST \
    -e POSTGRES_PORT=$POSTGRES_PORT \
    -e POSTGRES_SSLMODE=$POSTGRES_SSLMODE \
    -e CASDOOR_ENDPOINT=$CASDOOR_ENDPOINT \
    golang:1.24.1 \
    sh -c "go test -v -timeout=120s ./tests/auth/... 2>&1" || TEST_FAILED=1

echo ""
if [ "$TEST_FAILED" = "1" ]; then
    echo -e "${RED}❌ Tests failed!${NC}"
    echo -e "${YELLOW}💡 To view logs:${NC}"
    echo -e "   docker logs ocf-postgres-test"
    echo -e "   docker logs ocf-casdoor-test"
    echo -e "   docker logs ocf-casdoor-db-test"
    echo ""
    echo -e "${YELLOW}💡 Containers are still running for debugging.${NC}"
    echo -e "${YELLOW}   Run 'docker-compose -f docker-compose.test.yml down -v' to clean up when done.${NC}"
else
    echo -e "${GREEN}✅ All tests passed!${NC}"
    echo ""
    echo -e "${YELLOW}🧹 Cleaning up test containers...${NC}"
    docker-compose -f docker-compose.test.yml down -v
    echo -e "${GREEN}✨ Done!${NC}"
fi
