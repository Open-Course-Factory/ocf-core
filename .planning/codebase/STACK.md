# Technology Stack

**Analysis Date:** 2026-01-11

## Languages

**Primary:**
- Go 1.24.1 - All backend application code (`go.mod`)

**Secondary:**
- JavaScript/TypeScript - Build scripts, Marp/Slidev CLI tools (`package.json`)
- Markdown - Course generation content format

## Runtime

**Environment:**
- Go 1.24.1 - Backend runtime (`go.mod`)
- Node.js 21 - Frontend/tooling runtime (`Dockerfile`)
- Docker - Containerized deployment

**Package Manager:**
- Go Modules - `go.mod`, `go.sum`
- npm - `package.json` for Marp/Slidev presentation tools
- Multi-module monorepo pattern

## Frameworks

**Core:**
- Gin Web Framework v1.10.1 - HTTP server and routing (`github.com/gin-gonic/gin`)
- GORM v1.30.1 - Database ORM (`gorm.io/gorm`)

**Testing:**
- Go standard testing package - Unit and integration tests
- Testify/Assert - Assertion library (`github.com/stretchr/testify`)
- Testify/Mock - Mocking framework

**Build/Dev:**
- Swaggo v1.16.6 - API documentation generation (`github.com/swaggo/swag`)
- Docker Compose - Multi-service orchestration

## Key Dependencies

**Critical:**
- Casdoor Go SDK v1.14.0 - OAuth/SSO authentication (`github.com/casdoor/casdoor-go-sdk`, `src/auth/casdoor/`)
- Casbin v2.120.0 - RBAC/ABAC authorization (`github.com/casbin/casbin/v2`)
- Stripe Go SDK v82.5.0 - Payment processing (`github.com/stripe/stripe-go/v82`, `src/payment/services/stripeService.go`)

**Infrastructure:**
- PostgreSQL Driver v1.6.0 - Database access (`gorm.io/driver/postgres`, `src/db/global_db.go`)
- Gorilla WebSocket v1.5.3 - Real-time terminal connections (`github.com/gorilla/websocket`)
- Go-Git v5.16.2 - Git repository operations for course imports (`github.com/go-git/go-git/v5`)

**Frontend/Generation:**
- Marp Core ^3.7.0 - Markdown presentation rendering (`@marp-team/marp-core`)
- Slidev CLI ^51.6.0 - Modern presentation framework (`@slidev/cli`)
- Markdown-it ^13.0.1 - Markdown parsing (`markdown-it`)

## Configuration

**Environment:**
- .env files - Development configuration (`.env`, `.env.dist`, `.env.test`)
- Environment variables - Runtime configuration via `os.Getenv()`
- Feature flags - `FEATURE_COURSES_ENABLED`, `FEATURE_LABS_ENABLED`, `FEATURE_TERMINALS_ENABLED` (`src/configuration/featureFlags.go`)

**Build:**
- Makefile - Build automation and test targets
- Docker Compose - Service orchestration (`docker-compose.yml`, `docker-compose.test.yml`)
- JWT key - `token_jwt_key.pem` for authentication

## Platform Requirements

**Development:**
- Linux/macOS/Windows - Any platform with Go 1.24.1 and Docker
- Docker - Required for PostgreSQL, MySQL (Casdoor), PgAdmin services

**Production:**
- Docker containers - Multi-service architecture
  - ocf-core - Main Go application
  - postgres:16-alpine - PostgreSQL database
  - casbin/casdoor - Identity provider
  - mysql:8.0.25 - Casdoor database
  - dpage/pgadmin4 - Database administration
  - Custom Slidev image - Presentation generation

---

*Stack analysis: 2026-01-11*
*Update after major dependency changes*
