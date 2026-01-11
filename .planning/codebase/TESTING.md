# Testing Patterns

**Analysis Date:** 2026-01-11

## Test Framework

**Runner:**
- Go standard testing package
- Makefile test automation

**Assertion Library:**
- testify/assert - Fluent assertion library (`github.com/stretchr/testify/assert`)
- Matchers: `assert.NoError()`, `assert.Equal()`, `assert.NotNil()`, `assert.True()`

**Mocking:**
- testify/mock - Mock framework with expectations (`github.com/stretchr/testify/mock`)

**Run Commands:**
```bash
make test                          # All tests with race detection (-race -timeout=30s)
make test-unit                     # Unit tests only (-short flag)
make test-integration              # Integration tests (-timeout=60s)
make test-entity-manager           # Entity management module tests
make coverage                      # Coverage report (coverage.out)
make coverage-html                 # HTML coverage report (coverage.html)
make benchmark                     # Performance benchmarks (-bench=. -benchmem)
make test-race                     # Race condition detection (-race -count=5)
make test-parallel                 # Parallel execution (-parallel=4)
```

## Test File Organization

**Location:**
- `/workspaces/ocf-core/tests/` - Separate from source code
- Test packages: `{module}_tests` (e.g., `entityManagement_tests`, `payment_tests`)

**Naming:**
- Unit tests: `{feature}_test.go` (e.g., `genericService_test.go`)
- Integration tests: `integration_test.go`
- Benchmarks: `{feature}_benchmark_test.go` (e.g., `pagination_benchmark_test.go`)

**Structure:**
```
tests/
├── entityManagement/         # 1,300+ lines, 15+ test files
│   ├── genericService_test.go
│   ├── genericRepository_test.go
│   ├── integration_test.go
│   ├── selective_preloading_test.go
│   ├── benchmarks_test.go
│   └── testutils/            # Shared test utilities
├── payment/                  # Payment module tests
│   ├── subscriptionService_test.go
│   └── integration/
├── terminalTrainer/          # Terminal trainer tests
├── courses/                  # Course functionality tests
├── auth/                     # Auth tests
└── testTools/                # Shared testing utilities
```

## Test Structure

**Suite Organization:**
```go
import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestGenericService_CreateEntity_Success(t *testing.T) {
    // Setup
    mockRepo := new(MockGenericRepository)
    service := NewGenericService(mockRepo)

    // Configure mock
    mockRepo.On("CreateEntity", mock.Anything, "Entity").Return(entity, nil)

    // Execute
    result, err := service.CreateEntity(input, "Entity")

    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, result)
    mockRepo.AssertExpectations(t)
}

func TestFeature_WithSubtests(t *testing.T) {
    t.Run("Scenario 1", func(t *testing.T) {
        // Test scenario 1
    })

    t.Run("Scenario 2", func(t *testing.T) {
        // Test scenario 2
    })
}
```

**Patterns:**
- Subtests with `t.Run()` for grouping related tests
- Setup/execute/assert structure
- Mock expectations checked with `AssertExpectations(t)`

## Mocking

**Framework:**
- testify/mock - `github.com/stretchr/testify/mock`

**Patterns:**
```go
type MockGenericRepository struct {
    mock.Mock
}

func (m *MockGenericRepository) CreateEntity(data any, entityName string) (any, error) {
    args := m.Called(data, entityName)
    return args.Get(0), args.Error(1)
}

// Usage in tests:
mockRepo := new(MockGenericRepository)
mockRepo.On("CreateEntity", mock.Anything, "Course").Return(course, nil)
result, err := mockRepo.CreateEntity(input, "Course")
assert.NoError(t, err)
mockRepo.AssertExpectations(t)
```

**What to Mock:**
- Repository interfaces (database access)
- External service clients (Casdoor, Stripe, Terminal Trainer)
- HTTP clients
- Time/date functions (not commonly mocked in this codebase)

**What NOT to Mock:**
- Pure functions
- Internal business logic
- DTOs and models

**Mock Locations:**
- `src/auth/mocks/` - Casdoor service mocks
- `src/worker/services/mockWorkerService.go` - Worker service mock
- Test files: Inline mock definitions

## Fixtures and Factories

**Test Data:**
```go
// Factory pattern for test entities
func NewTestID() uuid.UUID {
    return uuid.New()
}

func NewTestIDString() string {
    return uuid.New().String()
}

// Builder pattern (tests/entityManagement/testutils/builders.go)
func NewCourseBuilder() *CourseBuilder {
    return &CourseBuilder{
        course: &models.Course{
            BaseModel: models.BaseModel{ID: NewTestID()},
            Name: "Test Course",
        },
    }
}
```

**Location:**
- Factory functions: Defined in test files near usage
- Test utilities: `tests/testTools/`, `tests/{module}/testutils/`
- Shared fixtures: `tests/{module}/testutils/setup.go`

## Coverage

**Requirements:**
- No enforced coverage target
- Coverage tracked for awareness via Makefile

**Configuration:**
- Tool: Go standard coverage (`go test -coverprofile`)
- Output: `coverage.out` (text), `coverage.html` (visual)
- Excludes: Test files (`*_test.go`) automatically excluded

**View Coverage:**
```bash
make coverage               # Generate coverage.out
make coverage-html          # Generate coverage.html and open in browser
```

**Critical Gap:**
- Most payment and terminal trainer logic untested
- `src/payment/services/stripeService.go` (3,067 lines) - No tests
- `src/terminalTrainer/services/terminalTrainerService.go` (1,472 lines) - No tests

## Test Types

**Unit Tests:**
- Scope: Test single function/service in isolation
- Mocking: Mock all external dependencies (repositories, external services)
- Speed: Fast (<100ms per test)
- Examples: `genericService_test.go`, `userPermissionsService_test.go`

**Integration Tests:**
- Scope: Test multiple modules together with real database
- Mocking: Mock only external services (Stripe, Casdoor)
- Database: SQLite in-memory for fast tests
- Examples: `tests/entityManagement/integration_test.go`

**Benchmark Tests:**
- Framework: Go standard benchmarking
- Files: `*_benchmark_test.go`
- Run: `make benchmark`
- Examples: `pagination_benchmark_test.go`, `benchmarks_test.go`

**E2E Tests:**
- Not currently implemented
- Future consideration: API endpoint E2E tests

## Common Patterns

**Test Naming:**
```go
// Format: Test[Feature][Scenario][Expectation]
func TestGenericService_CreateEntity_Success(t *testing.T) { }
func TestGenericService_CreateEntity_RepositoryError(t *testing.T) { }
func TestSelectivePreloading_NoIncludes_EntityLoads(t *testing.T) { }
func TestSelectivePreloading_DeepNestedRelation_LoadsMultipleLevels(t *testing.T) { }
```

**Mock Setup:**
```go
mockRepo := new(MockGenericRepository)
mockRepo.On("GetEntity", entityID, "Course").Return(course, nil)

// Execute test
result, err := service.GetEntity(entityID, "Course")

// Verify mock was called as expected
mockRepo.AssertExpectations(t)
```

**Error Testing:**
```go
// Test error cases
mockRepo.On("GetEntity", entityID, "Course").Return(nil, gorm.ErrRecordNotFound)
result, err := service.GetEntity(entityID, "Course")
assert.Error(t, err)
assert.Nil(t, result)
```

**Database Testing:**
```go
// In-memory SQLite for fast unit tests
db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
db.AutoMigrate(&models.Course{})

// Test with real database
repo := NewGenericRepository(db)
result, err := repo.CreateEntity(course, "Course")
assert.NoError(t, err)
```

## CI/CD Integration

**GitLab CI Configuration:** `.gitlab-ci.yml`

**Stages:**
- check - Linting and code quality
- test - Unit and integration tests
- build - Build binaries and containers
- release - Tag and publish releases

**Test Jobs:**
- Unit tests with PostgreSQL service
- Race condition detection (`-race` flag)
- Integration tests (60s timeout)
- Authentication tests
- Coverage reporting

**Database Setup:**
- PostgreSQL 16 service in CI
- Environment variables: `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`
- Test configuration: `.env.test`

**Caching:**
- Go modules cache per branch
- Speeds up CI runs

## Test Utilities

**Shared Test Context:**
File: `tests/entityManagement/testutils/setup.go`
```go
type TestContext struct {
    DB      *gorm.DB
    Entity  any
    Cleanup func()
}

func SetupEntityTest(t *testing.T, entity any) *TestContext {
    // Creates in-memory SQLite database
    // Initializes entity registry
    // Sets up hook system in test mode
    // Provides cleanup function
}
```

**Custom Assertions:**
File: `tests/entityManagement/testutils/assertions.go`
- `AssertNoError(t, err)` - Fail if error
- `AssertEqual(t, expected, actual)` - Deep equality
- Standard testify/assert library used

**Test Data Builders:**
File: `tests/entityManagement/testutils/builders.go`
```go
func NewTestID() uuid.UUID
func NewTestIDString() string
func NewTestSlice[T any](n int, builder func(int) T) []T
```

## Known Testing Gaps

**Untested Critical Code:**
- `src/payment/services/stripeService.go` (3,067 lines) - **HIGH PRIORITY**
- `src/payment/services/bulkLicenseService.go` (643 lines)
- `src/terminalTrainer/services/terminalTrainerService.go` (1,472 lines) - **HIGH PRIORITY**
- `src/organizations/services/importService.go` (536 lines)
- `src/courses/services/courseService.go` (525 lines)

**Test Count:**
- Only 5 test files found in source directories
- 15+ test files in `tests/entityManagement/` (1,300+ lines)
- Most feature modules lack comprehensive tests

**Recommendations:**
1. Add tests for payment module (Stripe integration critical)
2. Add tests for terminal trainer module (complex state management)
3. Add integration tests for full request/response cycles
4. Add E2E tests for critical user flows

---

*Testing analysis: 2026-01-11*
*Update when test patterns change*
