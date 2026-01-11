# Codebase Structure

**Analysis Date:** 2026-01-11

## Directory Layout

```
ocf-core/
├── main.go                        # Server entry point (221 lines)
├── go.mod                         # Go 1.24.1 module definition
├── package.json                   # Node.js dependencies (Marp, Slidev)
├── Makefile                       # Build targets and test automation
├── Dockerfile                     # Container build definition
├── docker-compose.yml             # Multi-service orchestration
├── .gitlab-ci.yml                 # CI/CD pipeline configuration
│
├── src/                           # Main source code (296 Go files)
│   ├── auth/                      # Authentication & authorization module
│   ├── entityManagement/          # Generic entity framework
│   ├── courses/                   # Course management module
│   ├── terminalTrainer/           # SSH terminal trainer module
│   ├── payment/                   # Stripe subscriptions & billing
│   ├── groups/                    # Class groups module
│   ├── organizations/             # Organization management
│   ├── email/                     # Email templates module
│   ├── configuration/             # Feature flags & config
│   ├── audit/                     # Audit logging
│   ├── generationEngine/          # Marp & Slidev integration
│   ├── worker/                    # Async job processing
│   ├── webSsh/                    # SSH terminal service
│   ├── cron/                      # Background cron jobs
│   ├── cli/                       # Command-line interface
│   ├── db/                        # Database connection
│   ├── initialization/            # Startup procedures
│   ├── middleware/                # Global middleware
│   ├── version/                   # Version endpoint
│   ├── utils/                     # Utility functions
│   └── html/                      # HTML templates
│
├── tests/                         # Test suite (separate from src/)
│   ├── entityManagement/          # 1,300+ lines, 15+ test files
│   ├── payment/                   # Payment module tests
│   ├── terminalTrainer/           # Terminal trainer tests
│   ├── courses/                   # Course functionality tests
│   ├── organizations/             # Organization tests
│   ├── auth/                      # Auth tests
│   └── testTools/                 # Shared test utilities
│
├── dist/                          # Build output (generated presentations)
├── courses/                       # Course content (Git repos)
├── themes/                        # Presentation themes (Marp/Slidev)
├── scripts/                       # Database migration scripts
└── docs/                          # API documentation (Swagger)
```

## Directory Purposes

**src/auth/**
- Purpose: Authentication via Casdoor OAuth and authorization via Casbin
- Contains: Auth controllers, middlewares, services, DTOs, models (UserSettings, SSHKeys)
- Key files: `authController.go` (OAuth callback), `authMiddleware.go` (JWT validation), `permissionService.go` (RBAC)
- Subdirectories: casdoor/, routes/, services/, repositories/, models/, dto/, hooks/, entityRegistration/, interfaces/, mocks/

**src/entityManagement/**
- Purpose: Generic entity framework for automatic CRUD generation
- Contains: Generic controllers, services, repositories, hooks system, Swagger config
- Key files: `genericController.go` (universal HTTP handlers), `genericService.go` (universal business logic), `genericRepository.go` (universal data access)
- Subdirectories: routes/, services/, repositories/, models/, dto/, hooks/, interfaces/, converters/, swagger/, utils/

**src/courses/**
- Purpose: Course and content management
- Contains: Course entities (Course, Chapter, Section, Page), generation jobs, scheduling, themes
- Key files: `courseService.go`, `generationRoutes.go` (async generation), Markdown writers (Slidev/Marp)
- Subdirectories: models/, dto/, services/, repositories/, routes/, hooks/, entityRegistration/, testHelpers/

**src/terminalTrainer/**
- Purpose: SSH terminal trainer for interactive CLI training
- Contains: Terminal entities, session management, SSH keys, terminal sharing
- Key files: `terminalTrainerService.go` (1,472 lines), `terminalController.go` (1,392 lines), two-layer security middleware
- Subdirectories: models/, services/, routes/, middleware/, hooks/, entityRegistration/, dto/, repositories/

**src/payment/**
- Purpose: Stripe integration for subscriptions and billing
- Contains: Subscription plans, user subscriptions, usage metrics, invoices, webhooks
- Key files: `stripeService.go` (3,067 lines - monolithic), `subscriptionService.go`, webhook handlers
- Subdirectories: models/, services/, routes/, middleware/, repositories/, integration/, entityRegistration/, dto/, utils/

**src/groups/**
- Purpose: Class/student groups management
- Contains: Group and GroupMember entities
- Subdirectories: models/, services/, repositories/, hooks/, entityRegistration/, dto/

**src/organizations/**
- Purpose: Organization and member management (Phase 1 feature)
- Contains: Organization entities, CSV import service
- Key files: `importService.go` (CSV parsing and import), `csvParser.go`
- Subdirectories: models/, services/, routes/, controller/, hooks/, repositories/, entityRegistration/, dto/, utils/

**src/email/**
- Purpose: Email template management and SMTP sending
- Contains: Email template entities, template initialization, SMTP service
- Key files: `emailService.go`, `initTemplates.go`
- Subdirectories: models/, services/, routes/, entityRegistration/, dto/

**src/configuration/**
- Purpose: Feature flag system and configuration management
- Contains: Feature entities, module configuration interface
- Key files: `featureFlags.go`, `configuration.go`
- Subdirectories: models/, services/, repositories/, interfaces/, entityRegistration/, dto/

**src/audit/**
- Purpose: Audit trail logging for compliance and security
- Contains: Audit log entities, logging services
- Key files: `auditService.go` (log authentication, access, security events)
- Subdirectories: models/, services/, controllers/

**src/generationEngine/**
- Purpose: Course presentation generation via Marp and Slidev
- Contains: Docker-based generators for Marp and Slidev
- Subdirectories: slidev_integration/, marp_integration/

**src/worker/**
- Purpose: Asynchronous job processing via external worker service
- Contains: Worker service client, generation package service, mock worker
- Subdirectories: services/

**src/webSsh/**
- Purpose: SSH terminal WebSocket service
- Contains: SSH client models and services
- Subdirectories: models/, services/, routes/

**src/cron/**
- Purpose: Background scheduled jobs
- Contains: Webhook cleanup, audit log cleanup cron jobs

**src/cli/**
- Purpose: Command-line interface for bulk operations
- Contains: Course generator CLI

**src/db/**
- Purpose: Database connection configuration
- Contains: `global_db.go` (PostgreSQL connection setup)

**src/initialization/**
- Purpose: Application startup procedures
- Contains: Database migrations, entity registration, permission setup, Swagger initialization

**src/middleware/**
- Purpose: Global HTTP middleware
- Contains: CORS configuration

**src/utils/**
- Purpose: Shared utility functions
- Contains: Validation, error handling, HTTP client, logger

**src/version/**
- Purpose: API version endpoint
- Contains: Version controller

**tests/**
- Purpose: Comprehensive test suite (separate from source)
- Contains: Unit tests, integration tests, benchmarks
- Organization: Mirrors src/ structure, separate test packages

## Key File Locations

**Entry Points:**
- `main.go` - Server entry point, route registration, middleware setup (221 lines)
- `src/cli/course_generator.go` - CLI entry point for bulk course generation

**Configuration:**
- `.env`, `.env.dist`, `.env.test` - Environment variable configuration
- `go.mod` - Go module dependencies (Go 1.24.1)
- `package.json` - Node.js dependencies (Marp, Slidev)
- `Makefile` - Build targets (test, coverage, lint, etc.)
- `docker-compose.yml` - Multi-service orchestration
- `src/configuration/keymatch_model.conf` - Casbin authorization model
- `token_jwt_key.pem` - JWT signing key

**Core Logic:**
- `src/entityManagement/services/genericService.go` - Universal CRUD service (4 core methods)
- `src/entityManagement/repositories/genericRepository.go` - Universal data access with filtering
- `src/entityManagement/routes/genericController.go` - Universal HTTP handlers
- `src/auth/authMiddleware.go` - JWT validation middleware (38 lines)
- `src/payment/services/stripeService.go` - Stripe integration (3,067 lines)

**Testing:**
- `tests/entityManagement/` - 1,300+ lines across 15+ test files
- `Makefile` - Test targets: test, test-unit, test-integration, coverage, benchmark

**Documentation:**
- `docs/swagger.json` - Auto-generated OpenAPI spec
- `PASSWORD_RESET_SETUP.md` - Password reset documentation
- `TERMINAL_TRAINER_STATUS_FIX.md` - Terminal trainer bug fix documentation
- `SECURITY.md` - Security documentation

## Naming Conventions

**Files:**
- Controllers: `{entity}Controller.go` (e.g., `userController.go`)
- Services: `{entity}Service.go` (e.g., `courseService.go`)
- Repositories: `{entity}Repository.go` (e.g., `terminalRepository.go`)
- Models: `{entity}.go` (e.g., `course.go`)
- DTOs: `{entity}Dto.go` or `{entity}DTO.go` (e.g., `courseDto.go`, `currentUserDTO.go`)
- Routes: `{entity}Routes.go` (e.g., `courseRoutes.go`)
- Tests: `*_test.go` (e.g., `genericService_test.go`)
- Benchmarks: `*_benchmark_test.go`

**Directories:**
- Lowercase with underscores for compound names
- Plural for collections: services/, routes/, repositories/, models/, dto/
- Feature modules: auth/, courses/, payment/, terminalTrainer/, etc.

**Special Patterns:**
- `init*.go` - Initialization files (e.g., `initHooks.go`, `initTemplates.go`)
- `*Registration.go` - Entity registration files
- `mock*.go` or `*_mocks.go` - Mock implementations for testing

## Where to Add New Code

**New Entity:**
- Model: `src/{module}/models/{entity}.go`
- DTO: `src/{module}/dto/{entity}Dto.go`
- Registration: `src/{module}/entityRegistration/{entity}Registration.go`
- Hooks (optional): `src/{module}/hooks/{entity}Hooks.go`
- Custom routes (optional): `src/{module}/routes/{entity}Routes.go`
- Custom service (optional): `src/{module}/services/{entity}Service.go`

**New Feature Module:**
- Create directory: `src/{module}/`
- Subdirectories: models/, dto/, services/, repositories/, routes/, hooks/, entityRegistration/
- Register in: `src/initialization/entities.go`, `main.go`

**New Test:**
- Unit test: `tests/{module}/{feature}_test.go`
- Integration test: `tests/{module}/integration_test.go`
- Test utilities: `tests/testTools/` or `tests/{module}/testutils/`

**Utilities:**
- Shared helpers: `src/utils/`
- Module-specific utilities: `src/{module}/utils/`

## Special Directories

**dist/**
- Purpose: Generated course presentations (HTML, PDF)
- Source: Auto-generated by Marp/Slidev engines
- Committed: No (.gitignore)

**courses/**
- Purpose: Course content from Git repositories
- Source: Cloned via go-git from external repos
- Format: course.json + Markdown files

**themes/**
- Purpose: Presentation themes for Marp/Slidev
- Subdirectories: sdv/ (default theme), custom themes
- Committed: Yes

**scripts/**
- Purpose: Database migration SQL scripts
- Format: *.sql files
- Usage: Manual database schema changes

---

*Structure analysis: 2026-01-11*
*Update when directory structure changes*
