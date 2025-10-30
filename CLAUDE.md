# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Environment

**IMPORTANT**: This project runs in a Docker Dev Container with Docker-in-Docker (DinD) enabled.

### Architecture

The dev container and application services run as **sibling containers** (not parent-child):
- Dev container: Your workspace where code runs
- Service containers: postgres, casdoor, casdoor_db, pgadmin
- All containers share the `devcontainer-network` Docker network

### Accessing Services from Dev Container

**Within the dev container** (where you run `go run main.go`, tests, etc.), access services using their **service names** as hostnames:

- **PostgreSQL**: `postgres:5432`
- **Casdoor**: `casdoor:8000`
- **Casdoor MySQL**: `casdoor_db:3306`
- **pgAdmin**: `pgadmin:80`

**Do NOT use `localhost` or `127.0.0.1`** - these refer to the dev container itself, not the sibling containers.

### Docker Commands from Dev Container

- `docker compose ps` - Won't show services (runs in different context)
- `docker ps` - Won't show services (sibling containers)
- Services are already running and accessible via service names
- No need to start/stop services manually - they persist across dev container sessions

### Port Mapping

Host machine can access services via localhost:
- `localhost:8080` → ocf-core API
- `localhost:5432` → postgres
- `localhost:8000` → casdoor
- `localhost:8888` → pgadmin

**Network flow**: `Host :8080` → `Dev Container :8080` → `Service Container :8080`

### Database Configuration

**Development Database** (`.env`):
- Host: `postgres` (sibling container via docker-compose.yml)
- Port: 5432
- Database: `ocf`

**Test Database** (`.env.test`):
- **Functional tests** (auth, integration): Use the same dev postgres sibling container
  - Host: `postgres` (NOT `localhost`)
  - Port: 5432
  - Database: `ocf_test`
- **Unit tests** (entity management): Use in-memory SQLite
  - No external database required

The dev postgres container is shared between development and functional tests, with separate databases (`ocf` vs `ocf_test`) for isolation.

### Database Access via MCP

**Model Context Protocol (MCP) servers are configured for direct database access:**

Available MCP tools (automatically loaded):
- **`mcp__postgres__query`** - Query the main `ocf` database
- **`mcp__postgres-test__query`** - Query the test `ocf_test` database

**Use cases:**
- Inspect database state during debugging
- Verify data after operations
- Check table schemas and relationships
- Debug permission policies in database
- Analyze test data

**Example:**
```
"Show me all organizations in the database"
→ Uses mcp__postgres__query

"Check what's in the ocf_test database"
→ Uses mcp__postgres-test__query
```

**Important:** MCP queries are read-only for safety. Use Go code or migrations for write operations.

Configuration: See `.mcp.json` for MCP server setup (auto-configured in dev container).

### Other MCP Tools Available

Additional MCP servers are configured for enhanced development capabilities:

- **Docker MCP** - Docker container management and inspection
- **Git MCP** - Repository operations and history analysis
- **Filesystem MCP** - File system operations (already available via built-in tools)
- **Fetch MCP** - Web content fetching (already available via WebFetch tool)

These MCPs provide additional context and capabilities when needed. They're automatically loaded and available without configuration.

**IMPORTANT - SQLite Testing Best Practice**:
When writing tests that use SQLite in-memory databases, **ALWAYS use shared cache mode**:

```go
// ❌ WRONG - Each connection gets its own database
db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

// ✅ CORRECT - All connections share the same in-memory database
db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
```

**Why?** Services create their own repository instances with new DB connections. In `:memory:` mode, each connection gets a completely separate database. With `cache=shared`, multiple connections access the same shared in-memory instance, allowing services to see tables created in test setup.

**IMPORTANT - GORM Foreign Key Updates**:
When updating a foreign key relationship in GORM, you **MUST update BOTH the ID field AND the associated entity**:

```go
// ❌ WRONG - Only updating the foreign key ID won't save the relationship
userSub.SubscriptionPlanID = newPlan.ID
ss.repository.UpdateUserSubscription(userSub)

// ✅ CORRECT - Update both the ID and the associated entity
userSub.SubscriptionPlanID = newPlan.ID
userSub.SubscriptionPlan = *newPlan
ss.repository.UpdateUserSubscription(userSub)
```

**Why?** GORM tracks associations through both the foreign key field and the associated entity. When you only update the ID, GORM doesn't recognize the relationship change and won't persist it. You must also update the associated entity so GORM can properly save the relationship.

This applies to all GORM relationships:
- `BelongsTo` (e.g., `UserSubscription.SubscriptionPlan`)
- `HasOne` and `HasMany` associations
- `Many2Many` relationships

## Project Overview

OCF Core is the core API for Open Course Factory, a platform for building and generating courses with integrated labs and environments. The system supports both Marp and Slidev presentation engines, with a focus on reusable course content and templating systems.

### Test Credentials and API Authentication

For API testing and development:
- **Email**: `1.supervisor@test.com`
- **Password**: `testtest`
- **Login Endpoint**: `POST http://localhost:8080/api/v1/auth/login`

**Authentication Flow:**

1. **Login** (get JWT token):
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"1.supervisor@test.com","password":"test"}'
```

Response:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "scope": "read write"
}
```

2. **Use Token** for authenticated requests:

**Method: Provide the full token directly in the Authorization header**

First, get a fresh token and extract it:
```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"1.supervisor@test.com","password":"test"}' | python3 -m json.tool
```

Copy the `access_token` value from the response, then use it directly in your requests:
```bash
curl -s -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6ImRlbW9fY2VydGlmaWNhdGUiLCJ0eXAiOiJKV1QifQ..." \
  "http://localhost:8080/api/v1/organizations" | python3 -m json.tool
```

**Important Notes:**
- Always use the **complete token string** (starts with `eyJ...`, very long)
- Tokens expire after ~7 days - if you get "invalid token", get a fresh one
- Command substitution `$(...)` does NOT work reliably with the Bash tool

**Common API Endpoints:**
- `GET /api/v1/subscription-plans` - List all subscription plans
- `POST /api/v1/subscription-plans/sync-stripe` - Sync plans with Stripe (requires auth)
- `GET /api/v1/user-subscriptions/current` - Get current user's subscription (requires auth)
- `GET /api/v1/terminals/user-sessions` - Get user's terminal sessions (requires auth)
- `GET /swagger/` - Full API documentation

## Development Commands

### Testing
- `make test` - Run all tests
- `make test-unit` - Run unit tests only
- `make test-integration` - Run integration tests
- `make test-entity-manager` - Run entity manager specific tests
- `make coverage` - Generate test coverage report
- `make coverage-html` - Generate HTML coverage report
- `make benchmark` - Run performance benchmarks

### Code Quality
- `make lint` - Run golangci-lint code analysis
- `make pre-commit` - Full validation before commit (tests, coverage, lint)

### Server Operations
- `go run main.go` - Start the API server (runs on :8080)
- `docker compose up -d` - Start all services (database, auth, etc.)

### Documentation
- `swag init --parseDependency --parseInternal` - Generate Swagger API documentation
- Visit `http://localhost:8080/swagger/` for complete API documentation

**IMPORTANT:** The `docs/` folder is **RESERVED FOR SWAGGER ONLY**. It contains auto-generated API documentation.
- ✅ Place project documentation in the root directory (e.g., `TERMINAL_PRICING_PLAN.md`)
- ✅ Or create a separate folder like `documentation/` or `guides/`
- ❌ **NEVER** manually create files in `docs/` - they will be overwritten by `swag init`

## Using Agents and Commands

### When to Use Agents (`.claude/agents/`)

Agents are **specialized assistants with independent context windows** for complex analysis and investigation. Use agents when you need:

**Code Review & Quality:**
- **`review-pr`** - Comprehensive PR reviews with architecture validation, pattern compliance, security checks
- **`review-entity`** - Audit entity implementations for completeness and best practices
- **`security-scan`** - Scan for vulnerabilities (SQL injection, secrets exposure, auth issues)
- **`architecture-review`** - Validate clean architecture, identify circular dependencies, assess scalability

**Performance & Optimization:**
- **`performance-audit`** - Detect N+1 queries, missing indexes, memory leaks, optimization opportunities

**Debugging & Investigation:**
- **`debug-test`** - Systematic test failure analysis with OCF-specific issue detection
- **`check-permissions`** - Debug Casbin policies, trace permission flow, identify missing permissions

**Learning & Understanding:**
- **`explain`** - Deep explanations of how systems work (architecture, data flow, integration points)
- **`find-pattern`** - Show implementation examples from codebase (validation patterns, service patterns, etc.)

**How agents work:**
- Independent context windows (won't clutter your main conversation)
- Deep multi-step analysis and investigation
- Comprehensive reports with actionable recommendations
- Reference with file:line numbers for easy navigation

### When to Use Commands (`.claude/commands/`)

Commands are **immediate actions in your main conversation** for quick scaffolding and modifications. Use commands when you need:

**Essential Commands:**
- **`/pre-commit`** ⭐ - Comprehensive pre-commit validation (USE BEFORE EVERY COMMIT!)
- **`/new-entity`** - Scaffold new entities with model, DTOs, registration, tests
- **`/test`** - Smart test runner based on recent changes
- **`/refactor`** - Systematic refactoring with pattern consistency

**Quick Actions:**
- **`/api-test`** - Quick API endpoint testing with auto-authentication
- **`/migrate`** - Database migration handling
- **`/update-docs`** - Regenerate Swagger documentation
- **`/improve`** - Code improvement suggestions
- **`/enforce-patterns`** - Pattern compliance scanning

**Command characteristics:**
- Execute immediately in main conversation
- Direct code modifications and scaffolding
- Quick validation and testing
- Workflow automation

### Best Practices

1. **Use `/pre-commit` before every commit** - Your quality gate
2. **Use agents for analysis** - PR reviews, debugging, learning
3. **Use commands for actions** - Scaffolding, refactoring, quick tests
4. **Combine them** - Use agent insights to guide command actions

**Example workflow:**
```
1. Make code changes
2. Use debug-test agent if tests fail
3. Run /test command to verify fixes
4. Use review-pr agent for quality check
5. Run /pre-commit before committing
6. Use review-entity agent for final audit
```

See `.claude/agents/README.md` and `.claude/commands/README.md` for complete documentation.

## Architecture Overview

### Core Components

**Entity Management System**: Generic CRUD operations for all entities with automatic route generation and Swagger documentation. Located in `src/entityManagement/`.

**Authentication**: Casdoor-based JWT authentication with role-based permissions (Casbin). Certificate stored in `src/auth/casdoor/token_jwt_key.pem`.

**Course Generation**: Dual engine support for Marp and Slidev with Git repository integration for courses and themes.

**Payment System**: Stripe integration with subscription plans, feature-based usage limits, and role management. Usage metrics are conditionally created based on database feature flags AND the plan's `Features` array (see `MODULAR_FEATURES.md`).

**Terminal Trainer Integration**: External backend system for interactive terminal sessions. OCF Core acts as a proxy, managing user keys and session lifecycle while delegating actual terminal operations to Terminal Trainer backend.

**Terminal Sharing and Hiding System**: Users can share terminals with different access levels (read/write/admin) and hide inactive terminals from their interface. Hidden status is managed per user and persisted in the database.

**Organizations & Groups System**: GitLab-style multi-tenant architecture with hierarchical organizations and groups. Supports personal organizations (auto-created per user), organization memberships with roles (owner/manager/member), cascading permissions from organizations to groups, and parent-child group relationships. See `.claude/docs/ORGANIZATION_GROUPS_SYSTEM.md` for complete documentation.

**Bulk Import System**: CSV-based bulk import for organizations, supporting users, groups, and memberships. Features include dry-run validation, update existing users option, error reporting with row-level details, and organization limit checking. Endpoint: `POST /api/v1/organizations/{id}/import`. See `BULK_IMPORT_FRONTEND_SPEC.md` for complete specification.

### Key Directories

- `src/auth/` - Authentication, users, SSH keys, permission management
- `src/organizations/` - Organizations, organization members, bulk import
- `src/groups/` - Groups, group members, hierarchy management
- `src/courses/` - Course models, generation, sessions
- `src/entityManagement/` - Generic CRUD system, member management utilities
- `src/payment/` - Stripe payment processing, subscription management
- `src/webSsh/` - SSH client integration
- `src/terminalTrainer/` - Terminal session management, sharing, and hiding functionality
- `src/utils/` - Shared utilities (logger, errors, validation, permissions)
- `tests/` - Comprehensive test suite

### Logging

The project uses an environment-aware logging system located in `src/utils/logger.go`.

**Debug Levels:**
- `utils.Debug()` - Only shown in development (ENVIRONMENT=development)
- `utils.Info()` - Always shown
- `utils.Warn()` - Always shown
- `utils.Error()` - Always shown

**Usage:**
```go
import "soli/formations/src/utils"

// Debug messages (only in development)
utils.Debug("🔍 Processing webhook: %s", event.Type)

// Production messages
utils.Info("User %s subscribed to plan %s", userID, planID)
utils.Warn("Failed to sync usage limits: %v", err)
utils.Error("Critical error in payment processing: %v", err)
```

**Environment Control:**
Set `ENVIRONMENT=development` in `.env` to enable debug logs. In production, set `ENVIRONMENT=production` to hide debug messages.

### Database

PostgreSQL with GORM ORM. Auto-migration runs on startup. Development mode includes debug SQL logging and test data setup.

### Configuration

- `.env` - Environment variables for database, auth, payments, terminal integration
- `src/configuration/` - Casdoor auth config, RBAC models
- Docker Compose for development environment setup

**Terminal Trainer Integration Environment Variables:**
- `TERMINAL_TRAINER_URL` - Base URL of Terminal Trainer backend
- `TERMINAL_TRAINER_ADMIN_KEY` - Admin API key for user management
- `TERMINAL_TRAINER_API_VERSION` - API version (default: "1.0")
- `TERMINAL_TRAINER_TYPE` - Optional terminal type prefix for specialized routes

### API Structure

RESTful API with `/api/v1` prefix. Generic entity routes auto-generated through entity management system. Manual routes for complex operations like course generation.

### Authentication Flow

1. User authenticates via Casdoor
2. JWT token validated using certificate
3. Casbin enforces role-based permissions
4. Payment middleware checks subscription limits

## Entity Management System (Framework Core)

### Current Architecture

The Entity Management System provides automatic CRUD operations for all entities through a registration pattern. This is the foundation for the planned evolution into a full framework.

**Location**: `src/entityManagement/`

### How It Works

#### 1. Entity Registration Pattern

Each entity (Course, Terminal, Machine, etc.) implements the `RegistrableInterface`:

```go
// Located in: src/entityManagement/interfaces/registrableInterface.go
type RegistrableInterface interface {
    GetEntityRegistrationInput() EntityRegistrationInput
    EntityModelToEntityOutput(input any) (any, error)
    EntityInputDtoToEntityModel(input any) any
    GetEntityRoles() EntityRoles
}
```

**Registration files** follow pattern: `src/{module}/entityRegistration/{entity}Registration.go`

Example: `src/courses/entityRegistration/courseRegistration.go`

#### 2. Entity Registration Components

Each registration defines:

- **EntityInterface**: The GORM model (e.g., `models.Course{}`)
- **EntityConverters**: Conversion functions between DTOs and models
  - `ModelToDto`: Convert model to output DTO
  - `DtoToModel`: Convert input DTO to model
  - `DtoToMap`: Convert DTO to map for partial updates
- **EntityDtos**: Three DTO types
  - `InputCreateDto`: For POST requests
  - `InputEditDto`: For PATCH requests (partial updates)
  - `OutputDto`: For GET responses
- **EntityRoles**: Permission mapping (role → HTTP methods)
- **EntitySubEntities**: Related entities for cascade operations
- **SwaggerConfig**: Auto-generated API documentation config
- **RelationshipFilters**: Define filter paths through relationships (e.g., filter sections by courseId)

#### 3. Automatic Features

When an entity is registered in `main.go`, the system automatically provides:

1. **CRUD Routes**: GET, POST, PATCH, DELETE at `/api/v1/{entity-plural}/`
2. **Swagger Documentation**: Auto-generated OpenAPI specs
3. **Permission Setup**: Casbin policies based on `GetEntityRoles()`
4. **Pagination**: Offset-based and cursor-based pagination
5. **Filtering**: Query parameters mapped to database filters
6. **Selective Preloading**: Load relationships on demand via `?includes=relation1,relation2`
7. **Hook System**: BeforeCreate, AfterCreate, BeforeUpdate, etc.

#### 4. Generic Service Layer

**Location**: `src/entityManagement/services/genericService.go`

Provides reusable operations:
- `CreateEntity(inputDto, entityName)`: Create with hooks and validation
- `GetEntity(id, entityName, includes)`: Fetch with selective preloading
- `GetEntities(filters, page, pageSize, includes)`: List with pagination
- `GetEntitiesCursor(cursor, limit, filters, includes)`: Cursor pagination
- `EditEntity(id, entityName, updates)`: Partial updates (requires map[string]any)
- `DeleteEntity(id, entity, scoped)`: Soft/hard delete

**PATCH Implementation Details** (`src/entityManagement/routes/editEntity.go`):

The PATCH endpoint uses a multi-step process to handle partial updates correctly:

1. **Bind JSON** → Creates `map[string]interface{}` from request body
2. **Filter Empty Strings** → Removes empty string values (treats as "no change")
3. **mapstructure.Decode** → Converts map to typed DTO struct with pointer fields
4. **EntityDtoToMap** → Converts DTO to `map[string]any` (only non-nil pointers)
5. **GORM Updates** → Applies partial update to database

```go
// Example: Frontend sends
{"display_name": "New Name", "description": "", "expires_at": ""}

// Step 2: Empty strings removed
{"display_name": "New Name"}

// Step 3: Decoded to EditDto
EditGroupInput{DisplayName: &"New Name", Description: nil, ExpiresAt: nil}

// Step 4: Converted to map
map[string]any{"display_name": "New Name"}

// Step 5: GORM updates only display_name field
```

**Important Notes:**
- Empty strings in request = field not updated (removed before decoding)
- `nil` pointer fields = field not included in update map (if using custom EntityDtoToMap)
- `time.Time` fields parsed from ISO8601 strings via decode hook
- All EditDto fields should be pointers for proper partial updates
- **Fallback behavior**: If no custom `EntityDtoToMap` is provided, uses default mapstructure conversion (all decoded fields included in map)
- **Best practice**: Provide custom `EntityDtoToMap` when using pointer fields in EditDto for precise control over which fields are updated

#### 5. How to Add a New Entity

**Step 1**: Create the model in `src/{module}/models/{entity}.go`

**Step 2**: Create DTOs in `src/{module}/dto/{entity}Dto.go`

**IMPORTANT - DTO Tag Requirements:**

All DTOs **MUST** include both `json` and `mapstructure` tags for proper data binding and decoding:

```go
// ✅ CORRECT - Both json and mapstructure tags
type CreateEntityInput struct {
    Name        string     `json:"name" mapstructure:"name" binding:"required"`
    DisplayName string     `json:"display_name" mapstructure:"display_name" binding:"required"`
    ExpiresAt   *time.Time `json:"expires_at,omitempty" mapstructure:"expires_at"`
}

type EditEntityInput struct {
    Name        *string     `json:"name,omitempty" mapstructure:"name"`
    DisplayName *string     `json:"display_name,omitempty" mapstructure:"display_name"`
    ExpiresAt   *time.Time  `json:"expires_at,omitempty" mapstructure:"expires_at"`
}

// ❌ WRONG - Missing mapstructure tags (will cause PATCH to fail)
type EditEntityInput struct {
    Name        *string `json:"name,omitempty"` // Missing mapstructure tag!
    DisplayName *string `json:"display_name,omitempty"` // Missing mapstructure tag!
}
```

**Why Both Tags Are Required:**

1. **`json` tags**: Used by `gin.BindJSON()` to parse request body into a map
2. **`mapstructure` tags**: Used by `mapstructure.Decode()` to convert the map into the DTO struct
3. **Snake case mapping**: `mapstructure` doesn't automatically convert `display_name` → `DisplayName` without explicit tags

**For EditDto (PATCH requests):**
- Use pointer types (`*string`, `*int`, `*time.Time`) for optional fields
- This enables partial updates (only non-nil fields are updated)
- The `EntityDtoToMap` converter checks for non-nil pointers

**Step 3**: Create registration in `src/{module}/entityRegistration/{entity}Registration.go`

**EntityDtoToMap Converter (Optional):**

For PATCH requests, you can optionally provide a custom `EntityDtoToMap` converter. If not provided, the system uses a default mapstructure-based conversion.

```go
// Option 1: Provide custom DtoToMap (recommended for pointer fields)
EntityConverters: entityManagementInterfaces.EntityConverters{
    ModelToDto: s.EntityModelToEntityOutput,
    DtoToModel: s.EntityInputDtoToEntityModel,
    DtoToMap:   s.EntityDtoToMap,  // Custom converter for PATCH
}

// Option 2: Omit DtoToMap (system uses default mapstructure conversion)
EntityConverters: entityManagementInterfaces.EntityConverters{
    ModelToDto: s.EntityModelToEntityOutput,
    DtoToModel: s.EntityInputDtoToEntityModel,
    // DtoToMap omitted - uses AbstractRegistrableInterface.EntityDtoToMap
}
```

**When to provide custom EntityDtoToMap:**
- EditDto has pointer fields (`*string`, `*int`, `*time.Time`)
- You want to include only non-nil fields in the update
- You need custom field filtering logic

**Custom EntityDtoToMap example:**
```go
func (g GroupRegistration) EntityDtoToMap(input any) map[string]any {
    dto := input.(dto.EditGroupInput)
    updates := make(map[string]any)

    // Only include non-nil pointer fields
    if dto.DisplayName != nil {
        updates["display_name"] = *dto.DisplayName
    }
    if dto.Description != nil {
        updates["description"] = *dto.Description
    }

    return updates
}
```

**Step 3 (continued)**: Registration example:

```go
type EntityRegistration struct {
    entityManagementInterfaces.AbstractRegistrableInterface
}

func (s EntityRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
    return entityManagementInterfaces.EntitySwaggerConfig{
        Tag: "entities",
        EntityName: "Entity",
        GetAll: &entityManagementInterfaces.SwaggerOperation{...},
        // ... other operations
    }
}

func (s EntityRegistration) EntityModelToEntityOutput(input any) (any, error) {
    // Convert model to output DTO
}

func (s EntityRegistration) EntityInputDtoToEntityModel(input any) any {
    // Convert input DTO to model
}

func (s EntityRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
    return entityManagementInterfaces.EntityRegistrationInput{
        EntityInterface: models.Entity{},
        EntityConverters: entityManagementInterfaces.EntityConverters{
            ModelToDto: s.EntityModelToEntityOutput,
            DtoToModel: s.EntityInputDtoToEntityModel,
        },
        EntityDtos: entityManagementInterfaces.EntityDtos{
            InputCreateDto: dto.EntityInput{},
            OutputDto:      dto.EntityOutput{},
            InputEditDto:   dto.EditEntityInput{},
        },
    }
}
```

**Step 4**: Register in `main.go`:

```go
sqldb.DB.AutoMigrate(&models.Entity{})
ems.GlobalEntityRegistrationService.RegisterEntity(registration.EntityRegistration{})
```

**Step 5**: Run `swag init --parseDependency --parseInternal` to update API docs

### Framework Evolution Roadmap

**Vision**: Transform OCF Core into a reusable Go framework for building CRUD-heavy APIs with modules.

#### Phase 1: Config-Driven Entities (3-4 weeks)

**Goal**: Define entities via YAML instead of code

```yaml
# modules/courses/course.yaml
entity:
  name: Course
  model: models.Course
  permissions:
    member: "GET|POST"
    admin: "GET|POST|PATCH|DELETE"
  fields:
    name: string
    version: string
    title: string
  relationships:
    - type: hasMany
      entity: Chapter
      field: Chapters
```

**Deliverables**:
- YAML parser for entity definitions
- Code generator from YAML to registration structs
- Backward compatibility with existing code-based registrations

#### Phase 2: Module System (4-6 weeks)

**Goal**: Loadable, pluggable modules

```go
type Module interface {
    Name() string
    Version() string
    Init(container *DIContainer) error
    RegisterEntities() []EntityRegistration
    RegisterRoutes(router gin.IRouter)
    RegisterHooks() []Hook
    Migrate(db *gorm.DB) error
    Shutdown() error
}
```

**Target Directory Structure**:
```
framework/
├── core/          # Framework core (entity system, DI, app bootstrap)
├── entity/        # Current entityManagement code
├── auth/          # Auth provider interface
└── db/            # Database provider interface

modules/
├── courses/
│   ├── module.yaml      # Module metadata and entity configs
│   ├── module.go        # Module implementation
│   ├── entities/        # Entity registrations
│   ├── models/          # GORM models
│   ├── services/        # Custom business logic
│   └── migrations/      # SQL migrations
└── terminals/
```

**Deliverables**:
- Module loader (scan directories for `module.yaml`)
- Module lifecycle management
- Migration system per module
- Module dependency resolution

#### Phase 3: Dependency Injection (2-3 weeks)

**Goal**: Decouple services from direct instantiation

**Options**:
- `uber/dig` - Reflection-based DI
- `google/wire` - Code generation DI
- Custom DI container

**Deliverables**:
- DI container implementation
- Service registration via DI
- Module integration with DI

#### Phase 4: Provider Abstractions (3-4 weeks)

**Goal**: Support multiple databases, auth systems, etc.

```go
type DatabaseProvider interface {
    Connect(config DatabaseConfig) error
    GetDB() interface{}
    Migrate(models ...interface{}) error
}

type AuthProvider interface {
    Authenticate(credentials Credentials) (*User, error)
    ValidateToken(token string) (*User, error)
    Authorize(user *User, resource, action string) bool
}
```

**Deliverables**:
- Database provider interface (PostgreSQL, MySQL, SQLite, MongoDB)
- Auth provider interface (Casdoor, JWT, OAuth2, Basic)
- Payment provider interface (Stripe, PayPal, custom)

### Known Challenges

#### 1. Module Discovery (High Difficulty)

**Problem**: Go doesn't support dynamic plugin loading well (platform-dependent, version-sensitive)

**Solutions**:
- Option A: Config files point to Go package paths, use code generation
- Option B: Modules are compiled into the binary, toggled via config
- Option C: Use Go 1.8+ plugin system (Linux/macOS only, fragile)

**Recommended**: Option B for stability

#### 2. Config-to-Code Generation

**Problem**: YAML → Go code generation requires careful handling of types, relationships, validations

**Solutions**:
- Use Go templates for codegen
- Validate YAML schema before generation
- Support incremental generation (don't overwrite manual code)

#### 3. Backward Compatibility

**Problem**: Existing code must continue working during transition

**Strategy**:
- Keep `GlobalEntityRegistrationService` as-is
- Add YAML loader as alternative registration method
- Gradual migration, one module at a time

#### 4. Testing Framework in Isolation

**Problem**: Framework code shouldn't depend on business logic

**Strategy**:
- Create `tests/framework/` for framework-only tests
- Use test fixtures and mocks for business entities
- Separate unit tests (framework) from integration tests (full app)

### Development Guidelines for Framework Work

1. **Don't Break Existing Code**: All changes must maintain backward compatibility
2. **Start Small**: Convert one module (e.g., `courses`) to config-driven first
3. **Document Patterns**: Update this file as patterns emerge
4. **Test Extensively**: Framework bugs impact all modules
5. **Incremental Refactoring**: Small PRs, frequent merges

### Key Files for Framework Development

- `src/entityManagement/entityManagementService/entityRegistrationService.go` - Registration system
- `src/entityManagement/services/genericService.go` - Generic CRUD operations
- `src/entityManagement/repositories/genericRepository.go` - Database operations
- `src/entityManagement/hooks/hookRegistry.go` - Hook system
- `src/entityManagement/swagger/` - Auto-documentation system
- `main.go:156-171` - Current entity registrations (hardcoded)

## Permissions and Security System

### Overview

The system uses **Casbin with Casdoor** for authorization. Permissions are managed dynamically in code, NOT through static configuration files.

**Permission Management Patterns:**
1. **Generic Entity Permissions**: Defined in entity registration via `GetEntityRoles()` method
2. **Specific Route Permissions**: Added dynamically in service methods
3. **User-Specific Permissions**: Created when entities are created/shared

**Role Mappings:**
- `"student"` role maps to `"member"` role in the system
- Role hierarchy: `Guest < Member < MemberPro < GroupManager < Trainer < Organization < Admin`

### Permission Helper Utilities (ALWAYS USE THESE)

**Location**: `src/utils/permissions.go`

**All permission operations MUST use centralized utils helpers** instead of direct Casbin calls:

```go
// Standard permission pattern
opts := utils.DefaultPermissionOptions()
opts.LoadPolicyFirst = true  // Auto-load policy before operation
opts.WarnOnError = true      // Log warnings instead of failing

// Grant permissions
utils.AddPolicy(casdoor.Enforcer, userID, route, methods, opts)

// Revoke permissions
utils.RemovePolicy(casdoor.Enforcer, userID, route, method, opts)
```

**Available Helpers** (12 functions):
- `AddPolicy` / `RemovePolicy` - Individual permissions
- `RemoveFilteredPolicy` - Filter-based removal
- `AddGroupingPolicy` / `RemoveGroupingPolicy` - Role assignments
- `AddPoliciesToGroup` / `AddPoliciesToUser` - Batch operations
- `RemoveAllUserPolicies` / `ReplaceUserPolicies` - Bulk management
- `HasPermission` / `HasAnyPermission` - Permission checks

**Why use helpers?**
- Consistent error handling
- Automatic policy loading
- Logging and debugging
- Single source of truth (100% refactored codebase)

### Key Rules

1. **Custom routes need explicit permissions** - Routes like `/hide`, `/share` are NOT covered by generic entity permissions
2. **Always clean up permissions** - Remove permissions when entities are deleted
3. **Use exact route paths** - Permissions must match `ctx.FullPath()` format
4. **Load policy when needed** - Set `LoadPolicyFirst: true` when checking immediately after adding

### Debugging Permissions

**Use the `check-permissions` agent for:**
- Understanding what permissions a user/role has
- Debugging 403 Forbidden errors
- Tracing permission flow
- Identifying missing permissions

See `.claude/docs/REFACTORING_COMPLETE_SUMMARY.md` for complete documentation.

## Utilities & Helper Functions

### Error Handling (`src/utils/errors.go`)

Pre-defined error constructors for consistent error messages:

```go
utils.ErrEntityNotFound("Group", groupID)
utils.ErrEntityAlreadyExists("Organization", orgName)
utils.ErrPermissionDenied("Group", "delete")
utils.ErrLimitReached("Organization", maxMembers)
utils.ErrCannotRemoveOwner("Organization")
```

### Validation (`src/utils/validation.go`)

21 reusable validators:

```go
// Chainable validation
err := utils.ChainValidators(
    utils.ValidateStringNotEmpty(name, "name"),
    utils.ValidateStringLength(name, 3, 100, "name"),
    utils.ValidateUniqueEntityName(db, "groups", name, "name"),
)

// Other validators available:
// - ValidateEntityExists, ValidateNotOwner, ValidateIsOwner
// - ValidateLimitNotReached, ValidateActive, ValidateNotExpired
// - ValidateUUID, ValidateNonNegative, ValidatePositive
// - ValidateOneOf (enum validation)
```

### Generic Converters (`src/entityManagement/converters/genericConverter.go`)

Eliminates reflection boilerplate in entity registrations:

```go
func (r Registration) EntityModelToEntityOutput(input any) (any, error) {
    return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
        model := ptr.(*models.Entity)
        return dto.EntityOutput{...}, nil
    })
}
```

### Base DTOs (`src/entityManagement/dto/baseDtos.go`)

Standard base types for all DTOs:
- `BaseEntityDto` - ID + timestamps
- `BaseEditDto` - Common edit fields (IsActive, Metadata)
- `NamedEntityOutput` - Name + DisplayName + Description
- `OwnedEntityOutput` - Entity with OwnerUserID

## Recent Major Work

### 2025-01-27: Complete Refactoring (Phases 1-6)

**Status**: ✅ 100% Complete

- **12 permission helper functions** created in utils package
- **37 methods/handlers refactored** across 6 phases
- **~2,600 lines eliminated** through pattern consolidation
- **100% permission management coverage** - all direct Casbin calls refactored
- **Zero breaking changes** - full backward compatibility
- **Framework readiness**: 60% → 85%

See `.claude/docs/REFACTORING_COMPLETE_SUMMARY.md` for complete details.

### 2025-01-27: Organizations & Groups System (Phase 1)

**Status**: ✅ Complete

- **GitLab-style multi-tenant architecture** implemented
- **Personal organizations** auto-created for each user
- **Organization roles**: owner, manager, member
- **Group roles**: owner, admin, assistant, member
- **Cascading permissions**: org managers access all org groups
- **Hierarchical groups**: parent-child relationships
- **Bulk CSV import**: users, groups, and memberships

See `.claude/docs/ORGANIZATION_GROUPS_SYSTEM.md` for complete documentation.

## Important Notes & Quick Reference

### Development Workflow
- **Always run `/pre-commit` before every commit** - Your essential quality gate
- Run `swag init --parseDependency --parseInternal` after API changes (or use `/update-docs`)
- Use `/new-entity` for scaffolding new entities with full CRUD setup
- Use `/test` for smart test running based on recent changes

### Code Patterns (MUST FOLLOW)
- **Permission Management**: Use `utils.AddPolicy()` / `utils.RemovePolicy()` helpers (NEVER direct Casbin calls)
- **Error Handling**: Use `utils.ErrEntityNotFound()` and similar constructors (consistent error messages)
- **Validation**: Use `utils.ChainValidators()` and validation helpers (chainable validation)
- **DTOs**: Both `json` AND `mapstructure` tags required (PATCH will fail without both)
- **Testing**: Use `file::memory:?cache=shared` for SQLite tests (shared cache mode)

### Architecture Rules
- Entity registrations in main.go enable automatic CRUD operations
- Use generic entity management system for new entities when possible
- Handlers → Services → Repositories (clean architecture layers)
- Models have no business logic (data structures only)

### System Features
- **Organizations**: All users get personal organizations auto-created on registration
- **Groups**: Can be standalone or linked to organizations for cascading permissions
- **Payment System**: Enforces usage limits based on subscription tiers
- **Bulk Import**: CSV import available at `/api/v1/organizations/{id}/import` (owners/managers only)
- **Casdoor**: Requires separate certificate setup for JWT validation

### When You Need Help

**Use Agents:**
- **Testing issues?** Use `debug-test` agent
- **Permission problems?** Use `check-permissions` agent
- **Performance slow?** Use `performance-audit` agent
- **Security concerns?** Use `security-scan` agent
- **Need to understand a system?** Use `explain` agent
- **Looking for code examples?** Use `find-pattern` agent
- **Reviewing a PR?** Use `review-pr` agent
- **Auditing an entity?** Use `review-entity` agent
- **Architecture questions?** Use `architecture-review` agent

**Use MCP Tools:**
- **Database inspection?** Use `mcp__postgres__query` or `mcp__postgres-test__query`
- **Check database state?** Query the database directly via MCP
- **Verify permissions in DB?** Query Casbin policies table
- **Analyze test data?** Use postgres-test MCP

See `.claude/agents/README.md` for complete agent documentation and `.mcp.json` for MCP configuration.