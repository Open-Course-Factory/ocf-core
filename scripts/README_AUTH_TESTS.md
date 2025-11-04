# Auth Tests - Local Debugging Guide

## Overview

The auth tests run in a **completely isolated environment** that won't affect your development database or services.

### Isolation Details

| Component | Development | Test Environment | Safe? |
|-----------|-------------|------------------|-------|
| Network | `devcontainer-network` | `test-network` | ✅ Yes |
| PostgreSQL | `postgres:5432` | `ocf-postgres-test:5433` | ✅ Yes |
| Database | `ocf` | `ocf_test` | ✅ Yes |
| Volume | `postgres` | `postgres-test-data` | ✅ Yes |
| Casdoor | `casdoor:8000` | `ocf-casdoor-test:8000` | ✅ Yes |

## Prerequisites

Make sure you have the JWT test key in place:
```bash
# The key should be at:
./src/auth/casdoor/token_jwt_key.pem
```

## Quick Start

### Option 1: Run Tests (Full CI Flow)

This mirrors exactly what runs in GitLab CI:

```bash
./scripts/run-auth-tests-locally.sh
```

This will:
1. Clean up any old test containers
2. Start PostgreSQL and MySQL test databases
3. Start Casdoor test service
4. Run all auth tests
5. Clean up automatically on success
6. Leave containers running on failure for debugging

### Option 2: Start Services Only (Debug Mode)

If you want to debug manually or run tests step-by-step:

```bash
./scripts/start-test-services.sh
```

This will start all test services and leave them running for you to:
- Inspect logs
- Connect to the test database
- Run individual tests
- Debug issues

## Debugging Commands

### View Logs
```bash
# PostgreSQL logs
docker logs ocf-postgres-test

# Casdoor logs
docker logs ocf-casdoor-test

# MySQL logs
docker logs ocf-casdoor-db-test

# Follow logs in real-time
docker logs -f ocf-casdoor-test
```

### Connect to Test Database
```bash
# Using docker exec
docker exec -it ocf-postgres-test psql -U postgres -d ocf_test

# From your host (if test services are running)
psql -h localhost -p 5433 -U postgres -d ocf_test
# Password: postgres
```

### Check Service Health
```bash
docker-compose -f docker-compose.test.yml ps

# Or check health status
docker inspect ocf-postgres-test --format='{{.State.Health.Status}}'
docker inspect ocf-casdoor-test --format='{{.State.Health.Status}}'
```

### Run Individual Tests
```bash
# Make sure services are running first!
./scripts/start-test-services.sh

# Then run specific tests
docker run --rm \
    --network ocf-core_test-network \
    -v $(pwd):/workspace \
    -w /workspace \
    -e POSTGRES_DB=ocf_test \
    -e POSTGRES_USER=postgres \
    -e POSTGRES_PASSWORD=postgres \
    -e POSTGRES_HOST=ocf-postgres-test \
    -e POSTGRES_PORT=5432 \
    -e POSTGRES_SSLMODE=disable \
    -e CASDOOR_ENDPOINT=http://ocf-casdoor-test:8000 \
    golang:1.24.1 \
    sh -c "go test -v -run TestSpecificTest ./tests/auth/..."
```

## Cleanup

### Stop Services (Keep Data)
```bash
docker-compose -f docker-compose.test.yml down
```

### Stop Services + Delete Volumes (Full Cleanup)
```bash
docker-compose -f docker-compose.test.yml down -v
```

## Common Issues

### Issue: "Port already in use"
**Solution:** Check if your dev services are running on the same ports:
```bash
docker ps | grep -E "5433|8000"
# Stop conflicting containers if needed
```

### Issue: "Network not found"
**Solution:** The network is created automatically when you start services:
```bash
docker-compose -f docker-compose.test.yml up -d
```

### Issue: Tests fail with "connection refused"
**Solution:** Services might not be fully ready. Wait longer or check logs:
```bash
docker logs ocf-casdoor-test
# Look for "started" or "ready" messages
```

### Issue: JWT key missing
**Solution:** Ensure the test JWT key is in place:
```bash
ls -la ./src/auth/casdoor/token_jwt_key.pem
```

## CI vs Local Differences

The local scripts mirror the GitLab CI configuration with these differences:

| Aspect | GitLab CI | Local |
|--------|-----------|-------|
| JWT Key | From CI variable | Must be in place manually |
| Cleanup | Always automatic | Optional (manual cleanup available) |
| Go installation | Downloads Go 1.24 | Uses Docker image |
| Networking | DinD network | Host Docker network |

## What Gets Tested

The auth tests verify:
- Casdoor authentication flow
- JWT token validation
- User authentication endpoints
- Permission checks
- Integration between OCF and Casdoor

## Tips

1. **Keep services running between test runs** to speed up debugging - just use `./scripts/start-test-services.sh` once
2. **Check Casdoor logs first** - most auth issues originate from Casdoor configuration
3. **Use verbose test output** with `-v` flag to see detailed test execution
4. **Run specific tests** with `-run TestName` to focus on failing tests
5. **Clean volumes occasionally** - old test data can cause issues
