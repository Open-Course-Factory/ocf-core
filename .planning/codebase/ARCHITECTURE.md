# Architecture

**Analysis Date:** 2026-01-11

## Pattern Overview

**Overall:** Modular Monolith with Plugin-Based Entity Management

**Key Characteristics:**
- Modular monolith with independent, loosely-coupled feature modules
- Layered architecture: Controllers → Services → Repositories → Models
- Generic entity management framework for automatic CRUD generation
- Event-driven architecture using lifecycle hooks
- RBAC authorization via Casbin policy engine
- RESTful API with auto-generated Swagger documentation

## Layers

**API Layer (Routes & Controllers):**
- Purpose: HTTP request handling and response formatting
- Contains: Route registration, controllers, middleware application
- Locations: `src/*/routes/`, `src/*/controller/`
- Depends on: Service layer for business logic
- Used by: External clients (HTTP requests)
- Example: `src/courses/routes/courseRoutes/courseController.go`, `src/auth/routes/usersRoutes/userController.go`

**Service Layer:**
- Purpose: Core business logic and orchestration
- Contains: Business rules, validations, external service integration
- Locations: `src/*/services/`, `src/entityManagement/services/genericService.go`
- Depends on: Repository layer for data access, external APIs
- Used by: Controllers and other services
- Pattern: Interface-based design with dependency injection
- Example: `src/entityManagement/services/genericService.go` (universal CRUD), `src/payment/services/stripeService.go`

**Repository Layer (Data Access):**
- Purpose: Database operations and query management
- Contains: GORM-based database queries, filtering, pagination
- Locations: `src/*/repositories/`, `src/entityManagement/repositories/genericRepository.go`
- Depends on: GORM ORM, database connection
- Used by: Service layer
- Key abstraction: `GenericRepository` provides universal CRUD and filtering
- Example: `src/entityManagement/repositories/genericRepository.go`, `src/courses/repositories/courseRepository.go`

**Model Layer (Data Models):**
- Purpose: Entity definitions and database schema
- Contains: GORM models with JSON serialization tags
- Locations: `src/*/models/`, `src/entityManagement/models/baseModel.go`
- Base model: UUID primary keys (UUIDv7), OwnerIDs array, timestamps
- Example: `src/courses/models/course.go`, `src/payment/models/subscription.go`

**DTO Layer (Data Transfer Objects):**
- Purpose: Separate request/response schemas from internal models
- Contains: Input/Output DTOs, converters between models and DTOs
- Locations: `src/*/dto/`, `src/*/converters/`
- Example: `src/auth/dto/userDto.go` (UserInput, UserOutput), `src/courses/dto/courseDto.go`

## Data Flow

**Typical HTTP Request Flow (e.g., Create Course):**

1. HTTP POST /api/v1/courses
2. [CORS Middleware] - Validates origin (`src/middleware/corsMiddleware.go`)
3. [Auth Middleware] - Extracts JWT, validates token (`src/auth/authMiddleware.go:38`)
4. [Permission Check] - Casbin enforcer validates user role/permission (`src/auth/permissionService.go`)
5. [Payment Middleware] - Checks usage limits against subscription tier (`src/payment/middleware/usageLimitMiddleware.go`)
6. [Generic Controller] - Decodes request DTO, validates input (`src/entityManagement/routes/genericController.go`)
7. [Generic Service] - Creates entity via repository (`src/entityManagement/services/genericService.go:50-51`)
   - Pre-hooks execute (BeforeCreate) - synchronous blocking
   - Repository saves to database via GORM
   - Post-hooks execute (AfterCreate) - async fire-and-forget
8. [Response] - Serialized DTO returned as JSON

**Hook Execution Points:**

**Synchronous (blocking):**
- BeforeCreate - Validate, transform data before insert
- BeforeUpdate - Validate changes
- BeforeDelete - Check dependencies

**Asynchronous (fire-and-forget):**
- AfterCreate - Send notifications, initialize defaults
- AfterUpdate - Sync to external systems
- AfterDelete - Cleanup related data

**State Management:**
- Stateless HTTP server (no persistent in-memory state)
- Database-backed state via PostgreSQL
- JWT tokens for authentication state

## Key Abstractions

**Entity Management System:**
- Purpose: Auto-generate CRUD routes and services for any entity
- Components: `entityManagementService`, `genericService`, `genericController`, `genericRepository`
- Location: `src/entityManagement/`
- Pattern: Entity registration interface (`src/entityManagement/interfaces/registrableInterface.go`)
- Auto-generates: GET/POST/PATCH/DELETE routes at `/api/v1/{entity}`

**Hook Registry System:**
- Purpose: Execute custom code at entity lifecycle events
- Types: BeforeCreate, AfterCreate, BeforeUpdate, AfterUpdate, BeforeDelete, AfterDelete
- Location: `src/entityManagement/hooks/`
- Global registry: `hooks.GlobalHookRegistry`
- Example: `src/auth/hooks/initHooks.go` (user settings initialization)

**Role-Based Access Control (RBAC):**
- Framework: Casbin + GORM adapter
- Enforcer: `casdoor.Enforcer` (initialized in `main.go:76`)
- Permissions: Defined in entity registrations and `initialization/permissions.go`
- Middleware: `src/auth/authMiddleware.go` (JWT validation and permission checking)

**Payment & Usage Limits:**
- Subscription model: Free, Pro, Enterprise tiers
- Usage tracking: API calls, storage, entity limits
- Middleware: `src/payment/middleware/usageLimitMiddleware.go` (rate limiting)
- Integration: Stripe webhooks for subscription events

**Terminal Trainer Module:**
- Two-layer security model: Generic entity access (layer 1) + Terminal-specific permissions (layer 2)
- Entities: Terminal, Session, UserTerminalKey, TerminalShare
- Middleware: `src/terminalTrainer/middleware/terminalAccessMiddleware.go`
- Real-time: WebSocket connections for terminal I/O

**Audit Logging:**
- Events: Authentication, authorization, security, billing
- Service: `src/audit/services/auditService.go`
- Cleanup: Scheduled cron job (`src/cron/auditLogCleanup.go`)

## Entry Points

**Server Entry Point:**
- Location: `main.go` (221 lines)
- Responsibilities:
  1. Load environment variables from `.env`
  2. Initialize Casdoor authentication
  3. Initialize PostgreSQL database connection
  4. Run database migrations
  5. Setup Casbin permissions
  6. Initialize feature modules (payment, courses, terminals, etc.)
  7. Start background cron jobs
  8. Setup Gin router and CORS
  9. Register all routes
  10. Start HTTP server on port 8080

**CLI Entry Point:**
- Location: `src/cli/course_generator.go`
- Purpose: Bulk course generation from Git repositories
- Flags: -c (course name), -t (theme), -e (engine), -course-repo (Git URL), -slide-engine (slidev/marp), -user-id

**Database Initialization:**
- Location: `src/initialization/database.go`
- Function: `AutoMigrateAll(db *gorm.DB)` - Migrates all entity models
- Relationships: Creates join tables for many-to-many associations

**Entity Registration:**
- Location: `src/initialization/entities.go`
- Function: `RegisterEntities()` - Registers all entity types in global registry

## Error Handling

**Strategy:** Structured errors with HTTP status codes

**Patterns:**
- Services throw custom `EntityError` with HTTP status, code, message, details (`src/entityManagement/routes/errorHandler.go`)
- Controllers catch errors and map to HTTP responses
- Middleware catches unhandled errors as 500 Internal Server Error

**Error Types:**
- `EntityNotFoundError` - 404 Not Found
- `ValidationError` - 400 Bad Request
- `PermissionDeniedError` - 403 Forbidden
- `UnauthorizedError` - 401 Unauthorized

## Cross-Cutting Concerns

**Logging:**
- Standard Go log package
- Structured logging with context (user ID, entity type, action)
- Log at service boundaries and error points

**Validation:**
- GORM validation tags on model fields
- Gin binding validation on DTO inputs
- Custom validators: `src/utils/custom_validators.go`

**Authentication:**
- JWT tokens via Casdoor OAuth flow
- Bearer token in Authorization header
- Middleware: `src/auth/authMiddleware.go` extracts and validates tokens

**Authorization:**
- Casbin enforcer checks permissions
- Permission service: `src/auth/permissionService.go`
- Policy model: `src/configuration/keymatch_model.conf`

**Transaction Management:**
- GORM transactions for atomic operations
- Used in payment operations: `src/payment/repositories/paymentRepository.go`

---

*Architecture analysis: 2026-01-11*
*Update when major patterns change*
