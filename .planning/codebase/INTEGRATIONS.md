# External Integrations

**Analysis Date:** 2026-01-11

## APIs & External Services

**Authentication & Identity:**
- Casdoor - SSO/OAuth provider (`src/auth/casdoor/casdoorConnector.go`)
  - SDK/Client: casdoor-go-sdk v1.14.0
  - Auth: `CASDOOR_ENDPOINT`, `CASDOOR_CLIENT_ID`, `CASDOOR_CLIENT_SECRET`, `CASDOOR_ORGANIZATION_NAME`, `CASDOOR_APPLICATION_NAME`
  - Integration: User management, role synchronization (`src/payment/services/roleSync.go`), OAuth callback handling

**Payment Processing:**
- Stripe - Subscription billing and payments (`src/payment/services/stripeService.go`)
  - SDK/Client: stripe-go v82.5.0
  - Auth: `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PUBLISHABLE_KEY`
  - Endpoints used: Checkout sessions, customer portal, subscriptions, invoices, payment methods, webhooks
  - Webhook: `/webhooks/stripe` (`src/payment/routes/webhookRoutes.go`)

**Email:**
- SMTP (Scaleway) - Transactional emails (`src/email/services/emailService.go`)
  - Integration method: Standard SMTP protocol
  - Auth: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM_EMAIL`, `SMTP_FROM_NAME`
  - Templates: Password reset (`src/auth/services/passwordResetService.go`), template management (`src/email/services/initTemplates.go`)

**Terminal Services:**
- Terminal Trainer Service - Interactive terminal/CLI training (`src/terminalTrainer/services/terminalTrainerService.go`)
  - Integration method: REST API with admin key authentication
  - Auth: `TERMINAL_TRAINER_URL`, `TERMINAL_TRAINER_ADMIN_KEY`
  - Features: Session management, terminal sharing, SSH key management

**Worker Services:**
- OCF Worker Service - Asynchronous job processing (`src/worker/services/workerService.go`)
  - Integration method: HTTP polling
  - Auth: `OCF_WORKER_URL`, `OCF_WORKER_TIMEOUT`, `OCF_WORKER_RETRY_COUNT`, `OCF_WORKER_POLL_INTERVAL`
  - Mock: `src/worker/services/mockWorkerService.go`

## Data Storage

**Databases:**
- PostgreSQL 16 - Primary data store (`docker-compose.yml`, `src/db/global_db.go`)
  - Connection: `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
  - Client: GORM v1.30.1 with PostgreSQL driver
  - Container: postgres:16-alpine

- MySQL 8.0.25 - Casdoor identity provider database (`docker-compose.yml`)
  - Container: mysql:8.0.25
  - Used exclusively by Casdoor service

**File Storage:**
- Git repositories - Course content storage (`src/courses/routes/courseRoutes/createCourseFromGit.go`)
  - Client: go-git v5.16.2
  - Features: Clone repositories, import course content

## Authentication & Identity

**Auth Provider:**
- Casdoor - External SSO/OAuth provider
  - Implementation: OAuth2/OIDC flow via Casdoor Go SDK
  - Token storage: JWT in Authorization header (Bearer token)
  - Session management: JWT validation middleware (`src/auth/authMiddleware.go`)
  - Role synchronization: Automatic sync from Stripe subscriptions to Casdoor roles

**Authorization:**
- Casbin - RBAC/ABAC policy engine
  - Enforcer: Initialized in `main.go:76`
  - Policy file: `src/configuration/keymatch_model.conf`
  - Storage: Casbin GORM adapter v3.36.0

## Monitoring & Observability

**Audit Logging:**
- Custom audit system (`src/audit/services/auditService.go`)
  - Events: Authentication, authorization, security events, billing events
  - Cleanup: Scheduled cron job (`src/cron/auditLogCleanup.go`)
  - Storage: PostgreSQL database

**Logs:**
- Standard output - Application logs
  - No external log aggregation service integrated
  - Rely on container orchestration for log collection

## CI/CD & Deployment

**Hosting:**
- Docker Compose - Development and production deployment
  - Deployment: Manual via docker-compose up
  - Environment vars: Configured via .env files

**CI Pipeline:**
- GitLab CI - Automated testing and builds (`.gitlab-ci.yml`)
  - Stages: check, test, build, release
  - Test database: PostgreSQL service in CI
  - Secrets: GitLab CI variables

**Admin Tools:**
- PgAdmin 4 - PostgreSQL GUI (`docker-compose.yml`)
  - Port: 8888
  - Container: dpage/pgadmin4

## Environment Configuration

**Development:**
- Required env vars: Database credentials, Casdoor config, Stripe keys, SMTP settings, Terminal Trainer URL
- Secrets location: `.env` file (gitignored)
- Test mode: `.env.test` for test configuration

**Production:**
- Secrets management: Environment variables in container orchestration
- Database: PostgreSQL with persistent volumes

## Webhooks & Callbacks

**Incoming:**
- Stripe - `/webhooks/stripe` (`src/payment/routes/webhookRoutes.go`)
  - Verification: Signature validation via `stripe.webhooks.constructEvent`
  - Events: `payment_intent.succeeded`, `customer.subscription.*`, `invoice.*`, `customer.*`
  - Cleanup: Scheduled webhook event cleanup (`src/cron/webhookCleanup.go`)

**Outgoing:**
- None - No webhooks sent to external services

## Content Generation

**Presentation Tools:**
- Marp - Markdown to presentation conversion
  - Docker image: marpteam/marp-cli
  - Integration: `src/generationEngine/marp_integration/marp.go`

- Slidev - Modern presentation framework
  - Docker image: registry.gitlab.com/open-course-factory/ocf-core/ocf_slidev:latest
  - Integration: `src/generationEngine/slidev_integration/slidev.go`
  - npm dependencies: @slidev/cli ^51.6.0

---

*Integration audit: 2026-01-11*
*Update when adding/removing external services*
