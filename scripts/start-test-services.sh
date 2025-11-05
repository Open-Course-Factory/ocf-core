#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}🚀 Starting Test Services (Debug Mode)${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# Clean up any previous test containers
echo -e "${YELLOW}🧹 Cleaning up old test containers...${NC}"
docker-compose -f docker-compose.test.yml down -v --remove-orphans 2>/dev/null || true
echo ""

# Start all services
echo -e "${BLUE}🐳 Starting all test services...${NC}"
docker-compose -f docker-compose.test.yml up -d
echo ""

echo -e "${YELLOW}⏳ Waiting for services to be ready...${NC}"
echo -e "${YELLOW}   - PostgreSQL initializing...${NC}"
sleep 15
echo -e "${YELLOW}   - Casdoor initializing...${NC}"
sleep 10
echo ""

# Show service status
echo -e "${BLUE}📊 Service Status:${NC}"
docker-compose -f docker-compose.test.yml ps
echo ""

# Show service health
echo -e "${BLUE}🔍 Health Status:${NC}"
echo -e "  PostgreSQL: ${GREEN}$(docker inspect ocf-postgres-test --format='{{.State.Health.Status}}' 2>/dev/null || echo 'checking...')${NC}"
echo -e "  MySQL:      ${GREEN}$(docker inspect ocf-casdoor-db-test --format='{{.State.Health.Status}}' 2>/dev/null || echo 'checking...')${NC}"
echo -e "  Casdoor:    ${GREEN}$(docker inspect ocf-casdoor-test --format='{{.State.Health.Status}}' 2>/dev/null || echo 'checking...')${NC}"
echo ""

# Show useful info
echo -e "${BLUE}📝 Connection Info:${NC}"
echo -e "  PostgreSQL:  localhost:5433 (user: postgres, pass: postgres, db: ocf_test)"
echo -e "  Casdoor:     http://localhost:8000"
echo -e "  Network:     ocf-core_test-network"
echo ""

echo -e "${GREEN}✅ Test services are running!${NC}"
echo ""
echo -e "${YELLOW}💡 Useful commands:${NC}"
echo -e "  View logs:        docker logs ocf-postgres-test"
echo -e "                    docker logs ocf-casdoor-test"
echo -e "  Connect to DB:    docker exec -it ocf-postgres-test psql -U postgres -d ocf_test"
echo -e "  Stop services:    docker-compose -f docker-compose.test.yml down"
echo -e "  Stop & clean:     docker-compose -f docker-compose.test.yml down -v"
echo ""
echo -e "${YELLOW}💡 To run tests against these services:${NC}"
echo -e "  ./scripts/run-auth-tests-locally.sh"
echo ""
