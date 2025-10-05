# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Environment

**IMPORTANT**: This project runs in a Docker Dev Container with Docker-in-Docker (DinD) enabled. All Docker commands execute within the dev container and can start/stop services via docker-compose.

- Dev container has full docker and docker-compose access
- Services (postgres, casdoor, etc.) run as sibling containers
- Network: `devcontainer-network` for dev services, `test-network` for test services
- Port mapping: Host -> Dev Container -> Service Container

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

## Project Overview

OCF Core is the core API for Open Course Factory, a platform for building and generating courses with integrated labs and environments. The system supports both Marp and Slidev presentation engines, with a focus on reusable course content and templating systems.

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

## Architecture Overview

### Core Components

**Entity Management System**: Generic CRUD operations for all entities with automatic route generation and Swagger documentation. Located in `src/entityManagement/`.

**Authentication**: Casdoor-based JWT authentication with role-based permissions (Casbin). Certificate stored in `src/auth/casdoor/token_jwt_key.pem`.

**Course Generation**: Dual engine support for Marp and Slidev with Git repository integration for courses and themes.

**Payment System**: Stripe integration with subscription plans, usage limits, and role management.

**Lab Environment**: SSH-based remote environments with machine and connection management.

**Terminal Trainer Integration**: External backend system for interactive terminal sessions. OCF Core acts as a proxy, managing user keys and session lifecycle while delegating actual terminal operations to Terminal Trainer backend.

**Terminal Sharing and Hiding System**: Users can share terminals with different access levels (read/write/admin) and hide inactive terminals from their interface. Hidden status is managed per user and persisted in the database.

### Key Directories

- `src/auth/` - Authentication, users, groups, SSH keys
- `src/courses/` - Course models, generation, sessions
- `src/labs/` - Lab machines, connections, usernames
- `src/entityManagement/` - Generic CRUD system
- `src/payment/` - Stripe payment processing
- `src/webSsh/` - SSH client integration
- `src/terminalTrainer/` - Terminal session management, sharing, and hiding functionality
- `tests/` - Comprehensive test suite

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