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
- **Password**: `test`
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

**IMPORTANT**: Due to Bash tool limitations with command substitution, use the pre-approved TOKEN variable or provide the full token directly.

**Method 1: Use the pre-approved TOKEN variable (recommended for testing)**
```bash
# The $TOKEN variable is pre-configured in the approved tools list
curl -X GET http://localhost:8080/api/v1/terminals \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json"
```

**Method 2: Provide the full token directly (most reliable)**
```bash
# Use the complete token directly in the Authorization header
curl -X GET http://localhost:8080/api/v1/terminals \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6ImRlbW9fY2VydGlmaWNhdGUiLCJ0eXAiOiJKV1QifQ..." \
  -H "Content-Type: application/json" | python3 -m json.tool
```

**Method 3: Two-step approach (fallback)**
```bash
# Step 1: Get token and save to file
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"1.supervisor@test.com","password":"test"}' \
  | python3 -c "import sys, json; print(json.load(sys.stdin)['access_token'])" > /tmp/token.txt

# Step 2: Use token from file
curl -X GET http://localhost:8080/api/v1/terminals \
  -H "Authorization: Bearer $(cat /tmp/token.txt)" \
  -H "Content-Type: application/json"
```

**Note**: Avoid using `$()` command substitution in complex Bash commands as it can cause parsing issues with the Bash tool. When in doubt, use Method 1 or Method 2.

**Common API Endpoints:**
- `GET /api/v1/subscription-plans` - List all subscription plans
- `POST /api/v1/subscription-plans/sync-stripe` - Sync plans with Stripe (requires auth)
- `GET /api/v1/user-subscriptions/current` - Get current user's subscription (requires auth)
- `GET /api/v1/terminal-sessions/user-sessions` - Get user's terminal sessions (requires auth)
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

## Architecture Overview

### Core Components

**Entity Management System**: Generic CRUD operations for all entities with automatic route generation and Swagger documentation. Located in `src/entityManagement/`.

**Authentication**: Casdoor-based JWT authentication with role-based permissions (Casbin). Certificate stored in `src/auth/casdoor/token_jwt_key.pem`.

**Course Generation**: Dual engine support for Marp and Slidev with Git repository integration for courses and themes.

**Payment System**: Stripe integration with subscription plans, feature-based usage limits, and role management. Usage metrics are conditionally created based on database feature flags AND the plan's `Features` array (see `MODULAR_FEATURES.md`).

**Terminal Trainer Integration**: External backend system for interactive terminal sessions. OCF Core acts as a proxy, managing user keys and session lifecycle while delegating actual terminal operations to Terminal Trainer backend.

**Terminal Sharing and Hiding System**: Users can share terminals with different access levels (read/write/admin) and hide inactive terminals from their interface. Hidden status is managed per user and persisted in the database.

### Key Directories

- `src/auth/` - Authentication, users, groups, SSH keys
- `src/courses/` - Course models, generation, sessions
- `src/entityManagement/` - Generic CRUD system
- `src/payment/` - Stripe payment processing
- `src/webSsh/` - SSH client integration
- `src/terminalTrainer/` - Terminal session management, sharing, and hiding functionality
- `src/utils/` - Shared utilities (logger, helpers)
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
- `EditEntity(id, entityName, updates)`: Partial updates
- `DeleteEntity(id, entity, scoped)`: Soft/hard delete

#### 5. How to Add a New Entity

**Step 1**: Create the model in `src/{module}/models/{entity}.go`

**Step 2**: Create DTOs in `src/{module}/dto/{entity}Dto.go`

**Step 3**: Create registration in `src/{module}/entityRegistration/{entity}Registration.go`:

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

### Casbin/Casdoor Integration

The system uses Casbin with Casdoor for authorization. Permissions are managed dynamically in code, NOT through static configuration files.

**Permission Management Patterns:**
1. **Generic Entity Permissions**: Defined in entity registration files via `GetEntityRoles()` method
2. **Specific Route Permissions**: Added dynamically using `casdoor.Enforcer.AddPolicy()` in service methods
3. **User-Specific Permissions**: Created when entities are created/shared to allow specific users access to specific resources

### Role Mappings

- `"student"` role maps to `"member"` role in the system
- Role hierarchy: `Guest < Member < MemberPro < GroupManager < Trainer < Organization < Admin`

### Terminal Permissions

**Terminal hiding routes require specific permissions:**
- Terminal creation automatically adds hide permissions for owner
- Terminal sharing automatically adds hide permissions for recipient
- Permissions format: `userID, "/api/v1/terminal-sessions/{terminalID}/hide", "POST|DELETE"`

**Custom routes (like `/hide`) are NOT covered by generic entity permissions and require manual permission setup in service methods.**

### Authentication Middleware

`AuthManagement()` middleware checks Casbin permissions using:
- `ctx.FullPath()` - The exact route path with parameters
- `ctx.Request.Method` - HTTP method
- User roles from JWT token

## Important Notes

- Always run `swag init --parseDependency --parseInternal` after API changes
- Entity registrations in main.go enable automatic CRUD operations
- Use generic entity management system for new entities when possible
- Casdoor requires separate certificate setup for JWT validation
- Payment system enforces usage limits based on subscription tiers
- **Custom routes require manual Casbin permission setup in service methods**
- **Use `casdoor.Enforcer.AddPolicy(userID, route, method)` to grant specific permissions**