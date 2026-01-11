# Coding Conventions

**Analysis Date:** 2026-01-11

## Naming Patterns

**Files:**
- kebab-case or snake_case for all files (e.g., `genericService.go`, `user_permissions_test.go`)
- Test files: `*_test.go` (e.g., `genericService_test.go`, `selective_preloading_test.go`)
- Benchmark files: `*_benchmark_test.go` (e.g., `pagination_benchmark_test.go`)
- Mock files: `mock*.go` or `*_mocks.go` (e.g., `mockCasdoorService.go`, `shared_mocks.go`)
- DTO files: `*Dto.go` or `*DTO.go` (e.g., `userDto.go`, `currentUserDTO.go`)

**Functions:**
- camelCase for all functions (e.g., `createEntity`, `validateInput`)
- Exported (public): PascalCase (e.g., `CreateEntity`, `ValidateInput`)
- Constructor pattern: `New{Type}()` (e.g., `NewGenericService()`, `NewCourseBuilder()`)
- Interface methods: Action verbs (e.g., `GetEntity`, `SaveEntity`, `DeleteEntity`)

**Variables:**
- camelCase for variables (e.g., `userID`, `courseService`)
- Constants: UPPER_SNAKE_CASE (not commonly used in codebase)
- Receivers: Short abbreviations (e.g., `gs *genericService`, `ac *authController`)
- Error variables: Explicit names when needed (e.g., `decodeError`, `entityCreationError`) or standard `err`

**Types:**
- Interfaces: PascalCase, no `I` prefix (e.g., `GenericRepository`, `RegistrableInterface`)
- Structs: PascalCase (e.g., `GenericService`, `CourseRegistration`, `BaseModel`)
- DTOs: PascalCase with suffix (e.g., `CourseInput`, `UserOutput`, `CourseDTO`)
- Mocks: `Mock{Type}` prefix (e.g., `MockGenericRepository`, `MockCasdoorService`)

## Code Style

**Formatting:**
- Go standard formatting via `gofmt` (implicit, no custom config)
- Tabs for indentation (Go standard)
- No line length limit enforced (Go convention: ~80-120 chars)
- Consistent spacing around operators and after commas

**Imports:**
- Standard library imports first
- Third-party imports second
- Internal imports third
- Blank line between groups
- Example from `src/entityManagement/services/genericService.go`:
  ```go
  import (
      "errors"
      "fmt"

      "github.com/gin-gonic/gin"
      "gorm.io/gorm"

      "soli/formations/src/entityManagement/models"
      entityErrors "soli/formations/src/entityManagement/errors"
  )
  ```

**Linting:**
- No explicit linter configuration found (eslintrc, golangci-lint config)
- Makefile target: `make lint` (likely runs `golangci-lint run`)

## Import Organization

**Order:**
1. Standard library (e.g., `errors`, `fmt`, `time`)
2. Third-party packages (e.g., `github.com/gin-gonic/gin`, `gorm.io/gorm`)
3. Internal modules (e.g., `soli/formations/src/entityManagement/...`)

**Grouping:**
- Blank line between groups
- Alphabetical within each group (implicit Go convention)

**Aliasing:**
- Used to avoid conflicts: `entityErrors "soli/formations/src/entityManagement/errors"`
- Common alias: `entityManagementModels` for base models

## Error Handling

**Patterns:**
- Return errors, don't panic (except initialization code)
- Custom error types with HTTP status: `EntityError` (`src/entityManagement/errors/`)
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Explicit error variable names for clarity: `decodeError`, `entityCreationError`

**Error Types:**
- Structured errors with HTTP status codes, error codes, messages, details
- Example: `EntityNotFoundError`, `ValidationError`, `PermissionDeniedError`
- Handler: `src/entityManagement/routes/errorHandler.go`

**Critical Issue:**
- **19 instances of `panic()` and `log.Fatal()` in production code** - should use error returns instead
- Locations: `src/entityManagement/`, `src/auth/`, `src/generationEngine/`, `src/db/`, `src/courses/`

## Logging

**Framework:**
- Standard Go `log` package
- No structured logging library (e.g., logrus, zap)

**Patterns:**
- `log.Printf()` for informational messages
- `log.Fatalf()` for critical errors (crashes process)
- Context logging: Include user ID, entity type, action in messages

**When:**
- Log at service boundaries (controller entry, service calls, repository operations)
- Log errors with context before returning
- Log authentication events, authorization failures, security events

## Comments

**When to Comment:**
- Explain "why" not "what" (Go convention)
- Document business rules and complex logic
- Add Godoc comments for exported functions/types
- Note TODOs with context: `// TODO: Description of what needs to be done`

**Godoc:**
- Required for exported functions and types
- Format: Comment immediately preceding declaration
- Example from `src/entityManagement/repositories/genericRepository.go`:
  ```go
  // getFilterManager creates a filter manager configured for the given entity.
  // It retrieves registered relationship filters for the entity and initializes
  // the manager with all standard filter strategies.
  func (o *genericRepository) getFilterManager(entityName string) *filters.FilterManager {
  ```

**Swagger/Swag Documentation:**
- API endpoints use `@` directives in comments
- Example from `src/auth/authController.go`:
  ```go
  // Callback godoc
  //
  //  @Summary      Callback
  //  @Description  callback pour casdoor
  //  @Tags         callback
  //  @Accept       json
  //  @Produce      json
  //  @Success      200
  //  @Failure      404  {object}  errors.APIError
  //  @Router       /auth/callback [get]
  ```

**TODO Comments:**
- 13 TODO items found in codebase
- Format: `// TODO: Description` (no username or issue number)
- Examples: Dynamic permission querying, email notifications, Casdoor syncing

## Function Design

**Size:**
- No strict limit, but large files identified as technical debt:
  - `stripeService.go` (3,067 lines) - needs refactoring
  - `terminalTrainerService.go` (1,472 lines) - needs refactoring

**Parameters:**
- Interface-based for testability (e.g., `db *gorm.DB`, `repo GenericRepository`)
- Context passing: `ctx *gin.Context` for HTTP handlers
- Dependency injection via constructor functions

**Return Values:**
- Explicit error returns: `(result any, err error)`
- Early returns for guard clauses
- Named return values for complex functions

## Module Design

**Exports:**
- Named exports for all public APIs
- No default exports (not applicable in Go)
- Interface-based design for testability

**Package Structure:**
- Each feature module has consistent subdirectories: models/, dto/, services/, repositories/, routes/, hooks/, entityRegistration/
- Internal packages use lowercase package names matching directory names

**Dependencies:**
- Avoid circular dependencies
- Import from parent modules allowed, not from sibling modules (use interfaces)

## Validation

**GORM Tags:**
- 2,323 instances of validation tags found (binding:"required", min, max, oneof, etc.)
- Example: `json:"name" binding:"required,min=3,max=100"`

**Gin Binding:**
- Input validation via Gin's `ShouldBindJSON()` in controllers
- DTO structs define validation rules

**Custom Validators:**
- Located in `src/utils/custom_validators.go`
- Registered with Gin validator

## Test Patterns

**Test Naming:**
- `Test[Feature][Scenario][Expectation]` format
- Examples: `TestGenericService_CreateEntity_Success`, `TestSelectivePreloading_NoIncludes_EntityLoads`

**Test Structure:**
- Use `t.Run()` for subtests
- Setup/teardown pattern with test utilities
- Mock repositories with testify/mock

**Assertions:**
- testify/assert library: `assert.NoError()`, `assert.Equal()`, `assert.NotNil()`

---

*Convention analysis: 2026-01-11*
*Update when patterns change*
