# PostgreSQL Tests for Entity Management

This directory contains PostgreSQL-specific tests for the entity management system, particularly for testing many-to-many relationships and complex joins that require a real database.

## Overview

The test suite is divided into:

1. **SQLite Tests** - Fast, in-memory tests for basic CRUD operations
2. **PostgreSQL Tests** - Database-specific tests for:
   - Many-to-many relationships with join tables
   - Complex relationship filtering
   - Foreign key constraints
   - Transactions and concurrency
   - Advanced SQL features

## Files

### Test Files

- `postgres_test_utils.go` - PostgreSQL connection utilities and helpers
- `postgres_simple_test.go` - PostgreSQL-specific tests (CRUD, FK, M2M, transactions, concurrency)
- `relationships_test.go` - Original tests (some skipped for SQLite, use PostgreSQL to run them)
- `test_with_postgres.sh` - Quick test runner that starts PostgreSQL automatically

### Configuration Files

- `.gitlab-ci.yml` - GitLab CI pipeline with PostgreSQL service
- `run_tests_compiled.sh` - Local test runner script

## Running Tests Locally

### Prerequisites

You need a PostgreSQL instance running. Options:

#### Option 1: Docker Compose (Recommended)

```bash
# Start PostgreSQL
docker compose up -d postgres

# Or use docker directly
docker run -d \
  --name ocf-postgres-test \
  -e POSTGRES_DB=ocf_test \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  postgres:15-alpine
```

#### Option 2: Local PostgreSQL Installation

Install PostgreSQL and create a test database:

```sql
CREATE DATABASE ocf_test;
CREATE USER postgres WITH PASSWORD 'postgres';
GRANT ALL PRIVILEGES ON DATABASE ocf_test TO postgres;
```

### Environment Variables

Set these environment variables before running tests:

```bash
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=postgres
export POSTGRES_PASSWORD=postgres
export POSTGRES_DB=ocf_test
export POSTGRES_SSLMODE=disable
```

Or create a `.env.test` file:

```env
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_DB=ocf_test
POSTGRES_SSLMODE=disable
```

### Running the Tests

#### All Tests (SQLite + PostgreSQL)

```bash
# Using the test runner script
./tests/entityManagement/run_tests_compiled.sh all

# Or compile and run manually
go test -c -o entity_tests.exe ./tests/entityManagement
./entity_tests.exe -test.v
```

#### PostgreSQL Tests Only

```bash
# Using script
./tests/entityManagement/run_tests_compiled.sh

# Or manually
go test -v ./tests/entityManagement -run TestPostgres
```

#### Quick Tests (SQLite only, no PostgreSQL needed)

```bash
./tests/entityManagement/run_tests_compiled.sh quick
```

#### With Race Detector

```bash
./tests/entityManagement/run_tests_compiled.sh race
```

## GitLab CI/CD Pipeline

The GitLab CI pipeline automatically runs tests with PostgreSQL service.

### Pipeline Stages

1. **test:quick** - Fast SQLite tests for quick feedback
2. **test:entity-management** - Full test suite with PostgreSQL
3. **test:race** - Race condition detection
4. **build** - Build validation

### Pipeline Configuration

```yaml
# .gitlab-ci.yml
test:entity-management:
  services:
    - postgres:15-alpine

  variables:
    POSTGRES_DB: ocf_test
    POSTGRES_USER: postgres
    POSTGRES_PASSWORD: postgres_test_password

  script:
    - go test -v ./tests/entityManagement
```

### Viewing Results

- Coverage reports are generated as artifacts
- Test results appear in merge request discussions
- Coverage percentage is displayed in pipeline badges

## Test Structure

### PostgreSQL-Specific Tests

#### Basic CRUD (`TestPostgres_BasicCRUD`)

Tests basic Create, Read, Update, Delete operations with PostgreSQL-specific features like proper UUID handling.

#### Foreign Key Relationships (`TestPostgres_ForeignKeyRelationships`)

Tests one-to-many relationships using foreign keys:
- Course → Chapters (1:N)
- Chapter → Pages (1:N)
- Filtering pages by course via joins

#### Many-to-Many (`TestPostgres_ManyToManyJoinTable`)

Tests many-to-many relationships with join tables:
- Students ↔ Courses via `student_courses` table
- Bidirectional queries
- Association management

#### Transactions (`TestPostgres_TransactionSupport`)

Tests PostgreSQL transaction support:
- Commit on success
- Rollback on error
- ACID compliance

#### Concurrent Access (`TestPostgres_ConcurrentAccess`)

Tests concurrent database operations:
- Multiple goroutines writing simultaneously
- No data loss
- Proper isolation

### Relationship Filtering Tests

These tests validate the relationship filtering system used for querying entities through multiple join tables:

- `TestPostgres_RelationshipFilterPagesByCourse` - 3-level join
- `TestPostgres_RelationshipFilterPagesByChapter` - 2-level join
- `TestPostgres_RelationshipFilterPagesBySection` - Direct join
- `TestPostgres_RelationshipFilterWithMultipleIDs` - Multiple filter values
- `TestPostgres_ComplexRelationshipChain` - Complex hierarchy

## Troubleshooting

### Tests Are Skipped

If you see "PostgreSQL not available" messages:

1. Check PostgreSQL is running:
   ```bash
   docker ps | grep postgres
   # or
   pg_isready -h localhost -p 5432
   ```

2. Verify connection:
   ```bash
   psql -h localhost -U postgres -d ocf_test -c '\l'
   ```

3. Check environment variables:
   ```bash
   echo $POSTGRES_HOST
   ```

### Connection Refused

```bash
# Check PostgreSQL is listening
netstat -an | grep 5432

# Check firewall
sudo ufw status

# Try connecting directly
psql "host=localhost port=5432 user=postgres dbname=ocf_test sslmode=disable"
```

### Permission Denied

```sql
-- Grant necessary permissions
GRANT ALL PRIVILEGES ON DATABASE ocf_test TO postgres;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO postgres;
```

### Tests Hanging

The test suite uses pre-compilation to avoid `go test` hanging issues:

```bash
# Don't use this (may hang):
go test ./tests/entityManagement/...

# Use this instead:
go test -c -o tests.exe ./tests/entityManagement
./tests.exe -test.v
```

## CI/CD Integration

### GitLab CI

The `.gitlab-ci.yml` includes:
- PostgreSQL 15 Alpine service
- Automatic database setup
- Coverage reporting
- Artifact storage

### GitHub Actions (Future)

```yaml
services:
  postgres:
    image: postgres:15-alpine
    env:
      POSTGRES_DB: ocf_test
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    options: >-
      --health-cmd pg_isready
      --health-interval 10s
      --health-timeout 5s
      --health-retries 5
```

## Best Practices

1. **Always use transactions** for multi-step operations
2. **Clean up test data** after each test
3. **Use meaningful test names** with `TestPostgres_` prefix
4. **Skip gracefully** if PostgreSQL is not available
5. **Test both success and failure** scenarios
6. **Use table-driven tests** for similar scenarios

## Performance Notes

- SQLite tests: ~0.5 seconds total
- PostgreSQL tests: ~2-3 seconds total
- Race detector: ~5-10 seconds total

## Contributing

When adding new PostgreSQL tests:

1. Add test to `postgres_simple_test.go` or create new file
2. Use `SkipIfNoPostgres(t)` at the start
3. Clean up tables after test
4. Update this README if adding new test categories
5. Ensure CI pipeline passes

## Related Documentation

- [Entity Management System](../../src/entityManagement/README.md)
- [Relationship Filtering](../../src/entityManagement/docs/relationships.md)
- [Test Fixes Summary](./TEST_FIXES_SUMMARY.md)
