# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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

### Key Directories

- `src/auth/` - Authentication, users, groups, SSH keys
- `src/courses/` - Course models, generation, sessions
- `src/labs/` - Lab machines, connections, usernames
- `src/entityManagement/` - Generic CRUD system
- `src/payment/` - Stripe payment processing
- `src/webSsh/` - SSH client integration
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

## Important Notes

- Always run `swag init --parseDependency --parseInternal` after API changes
- Entity registrations in main.go enable automatic CRUD operations
- Use generic entity management system for new entities when possible
- Casdoor requires separate certificate setup for JWT validation
- Payment system enforces usage limits based on subscription tiers