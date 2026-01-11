# Codebase Concerns

**Analysis Date:** 2026-01-11

## Tech Debt

**Monolithic Service Files:**
- Issue: Extremely large service files with multiple responsibilities
- Files:
  - `src/payment/services/stripeService.go` - **3,067 lines** - Handles subscriptions, invoices, webhooks, bulk licenses, syncing
  - `src/terminalTrainer/services/terminalTrainerService.go` - **1,472 lines** - Terminal sessions, sharing, syncing, enum services
  - `src/terminalTrainer/routes/terminalController.go` - **1,392 lines** - Multiple endpoint handlers
  - `src/payment/routes/userSubscriptionController.go` - **1,233 lines** - Subscription management routes
- Why: Rapid feature development without refactoring
- Impact: High cognitive load, difficult to test, maintenance challenges, increased bug risk
- Fix approach: Split into domain-specific services (e.g., StripeSubscriptionService, StripeInvoiceService, StripeWebhookService)

**Error Handling with Panic/Fatal:**
- Issue: 19 instances of `panic()` and `log.Fatal()` in production code
- Files:
  - `src/entityManagement/services/genericService.go:468`
  - `src/entityManagement/routes/editEntity.go:82`
  - `src/auth/authController.go:49,54`
  - `src/auth/casdoor/casdoorConnector.go:34,46,52,58`
  - `src/generationEngine/slidev_integration/slidev.go:62,112,119,147,156`
  - `src/generationEngine/marp_integration/marp.go:66,71,124,133,141,150`
  - `src/db/global_db.go:53,58`
  - `src/configuration/configuration.go:31,37`
  - `src/courses/models/course.go:149,155`
- Why: Error handling shortcuts during development
- Impact: Any of these errors will crash the entire process
- Fix approach: Replace with proper error returns, graceful degradation, error recovery

**Missing Webhook Event Retry:**
- Issue: Webhook processing happens in async goroutine with no retry mechanism
- File: `src/payment/routes/webHookController.go:90-103`
- Why: Simplified async processing without considering failure scenarios
- Impact: Failed webhook events are lost permanently, can lead to inconsistent state between Stripe and database
- Fix approach: Implement persistent event queue (Redis, database table) with retry logic and dead letter queue

**Environment Variable Validation:**
- Issue: 37 instances of `os.Getenv()` without validation or fallback
- Files: Throughout codebase (configuration loading)
- Why: Assumed environment variables always exist
- Impact: Nil pointer dereferences or undefined behavior if required env vars missing
- Fix approach: Validate all required env vars on startup, fail fast with clear error message

## Known Bugs

**TODO Comments Indicate Incomplete Features:**
- `src/terminalTrainer/routes/userTerminalKeyController.go:77` - TODO: Récupérer le nom d'utilisateur depuis Casdoor
  - Symptoms: Terminal key creation may not properly associate username
  - Workaround: Manual username tracking
  - Root cause: Casdoor integration incomplete

- `src/auth/services/userSettingsService.go:94` - TODO: Send email notification about password change
  - Symptoms: Users not notified when password changes
  - Trigger: Password reset or update operations
  - Workaround: Users must manually verify password change
  - Root cause: Email notification not implemented

- `src/payment/services/stripeService.go:1337` - TODO: Send notification email/webhook to user about trial ending
  - Symptoms: Users not notified before trial expires
  - Trigger: Trial period approaching end
  - Workaround: Manual monitoring
  - Root cause: Trial end notification not implemented

**Test-Identified Issues:**
- `tests/entityManagement/integration_test.go:270` - TODO: IsActive field not being saved correctly
  - Symptoms: Boolean field handling inconsistency
  - Trigger: Entity creation with IsActive field
  - Root cause: GORM boolean field handling issue

**Terminal Trainer API Inconsistency:**
- From: `TERMINAL_TRAINER_STATUS_FIX.md`
  - Symptoms: Production API returns status as string ("0" or "1"), code expects int
  - Trigger: Terminal session status queries
  - Workaround: FlexibleInt custom unmarshal type
  - Root cause: Environment inconsistency between dev and prod API responses

## Security Considerations

**Environment File in Repository:**
- Risk: `.env` file may contain sensitive credentials (Stripe keys, Casdoor secrets, SMTP passwords)
- Files: `.env` (should be gitignored but may have been committed historically)
- Current mitigation: `.gitignore` includes `.env`, but file permissions show `-rwxrwxrwx` (777) locally
- Recommendations: Audit Git history for committed secrets, rotate any exposed credentials, use secret management service

**Two-Layer Security Model:**
- Area: Terminal trainer module (`src/terminalTrainer/middleware/terminalAccessMiddleware.go`)
- Risk: Complex security model with two permission layers (generic entity access + terminal-specific permissions)
- Files:
  - `src/terminalTrainer/routes/terminalRoutes.go` (Layer 2 security comments)
  - `src/terminalTrainer/middleware/terminalAccessMiddleware.go`
- Current mitigation: Well-documented in code comments
- Recommendations: Add integration tests for security boundary cases, document in architecture docs

**Password Reset Security:**
- Area: Password reset flow
- File: `src/auth/services/passwordResetService.go`
- Risk: Comment notes "Don't reveal if user exists or not (security best practice)" - good
- Current mitigation: Proper security best practice implemented
- Status: ✅ Well-designed

**Query Parameter Security:**
- Area: Authentication middleware
- File: `src/auth/authMiddleware.go`
- Risk: Comment warns "Query parameters are logged and visible in URLs (security risk)"
- Current mitigation: Uses Authorization header for bearer tokens instead
- Status: ✅ Correctly implemented

## Performance Bottlenecks

**N+1 Query Risk:**
- Problem: 130 files use `for...range` loops over database results
- Files:
  - `src/terminalTrainer/services/terminalTrainerService.go`
  - `src/organizations/services/organizationService.go`
  - `src/groups/services/groupService.go`
  - Multiple repository and service files
- Measurement: Not profiled (no performance benchmarks found)
- Cause: Potential eager loading not configured, sequential queries in loops
- Improvement path: Add GORM preloading/eager loading, identify and optimize hot paths with benchmarks

**Large Service Methods:**
- Problem: Methods exceeding 200+ lines in service files
- Files: `stripeService.go`, `terminalTrainerService.go`
- Cause: Complex business logic without decomposition
- Improvement path: Extract helper methods, split into smaller focused services

## Fragile Areas

**Stripe Integration:**
- File: `src/payment/services/stripeService.go` (3,067 lines)
- Why fragile: Monolithic service, complex webhook handling, multiple responsibilities
- Common failures: Webhook event processing failures, subscription state inconsistencies
- Safe modification: Add tests before changes, use transactions for multi-step operations
- Test coverage: ❌ No tests found

**Terminal Trainer Module:**
- Files: `src/terminalTrainer/services/`, `src/terminalTrainer/routes/`
- Why fragile: Complex state management (sessions, sharing, hiding), two-layer security, external API dependency
- Common failures: Session state inconsistencies, permission edge cases, API response format changes
- Safe modification: Thorough testing of state transitions, validate all API responses
- Test coverage: ⚠️ Only DTO tests and enum service tests

**Entity Management Hooks:**
- Files: `src/entityManagement/hooks/`, `src/*/hooks/`
- Why fragile: Async execution, implicit dependencies, test mode vs. production mode behavior
- Common failures: Hook execution order issues, async hook failures silently ignored
- Safe modification: Set test mode for synchronous execution, verify hook registration
- Test coverage: ⚠️ Limited hook testing found

## Missing Critical Features

**Webhook Event Retry System:**
- Problem: No persistent queue or retry mechanism for failed webhook processing
- Files: `src/payment/routes/webHookController.go`
- Current workaround: Events marked as processed before handling, failures only logged
- Blocks: Reliable payment event processing, data consistency
- Implementation complexity: Medium (requires queue system: Redis, database table with workers)

**Comprehensive Error Monitoring:**
- Problem: No centralized error tracking or monitoring service integration
- Current workaround: Logs to stdout, rely on manual log review
- Blocks: Proactive error detection, production debugging
- Implementation complexity: Low (integrate Sentry, Rollbar, or similar)

**Permission System Documentation:**
- Problem: Complex permission system (Casbin + entity-level + terminal two-layer) not fully documented
- Files: `src/auth/permissionService.go`, `src/terminalTrainer/middleware/`
- Current workaround: Code comments, scattered documentation
- Blocks: Onboarding, understanding security model
- Implementation complexity: Low (write architecture documentation)

## Test Coverage Gaps

**Payment Module:**
- What's not tested: Stripe integration, subscription lifecycle, webhook handling, invoice generation
- Files: `src/payment/services/stripeService.go` (3,067 lines), `src/payment/services/bulkLicenseService.go` (643 lines)
- Risk: Payment processing failures, data inconsistencies, subscription state bugs
- Priority: **CRITICAL**
- Difficulty to test: Medium (requires Stripe test fixtures, webhook simulation)

**Terminal Trainer Module:**
- What's not tested: Terminal session management, sharing logic, permission middleware
- Files: `src/terminalTrainer/services/terminalTrainerService.go` (1,472 lines)
- Risk: Session state bugs, permission bypass, data loss
- Priority: **HIGH**
- Difficulty to test: Medium (requires external API mocking)

**Course Management:**
- What's not tested: Course service, generation pipeline, Git import
- Files: `src/courses/services/courseService.go` (525 lines)
- Risk: Course generation failures, data corruption
- Priority: **MEDIUM**
- Difficulty to test: Medium (requires Git repo fixtures, Docker mocking)

**Organization Import:**
- What's not tested: CSV parsing, bulk import operations, transaction handling
- Files: `src/organizations/services/importService.go` (536 lines)
- Risk: Data import failures, partial imports, data corruption
- Priority: **MEDIUM**
- Difficulty to test: Low (CSV fixtures easy to create)

## Dependencies at Risk

**Stripe Go SDK:**
- Package: `github.com/stripe/stripe-go/v82` v82.5.0
- Risk: Currently on v82, check for newer versions (v83, v84)
- Impact: Missing features, potential security updates
- Migration plan: Review changelog, test in staging, upgrade incrementally

**Casdoor Go SDK:**
- Package: `github.com/casdoor/casdoor-go-sdk` v1.14.0
- Risk: Verify if latest version available
- Impact: OAuth/SSO functionality, user management
- Migration plan: Check for updates, test authentication flows

**GORM:**
- Package: `gorm.io/gorm` v1.30.1
- Risk: Check for v2.x updates
- Impact: Database operations, migrations
- Migration plan: Review breaking changes, comprehensive testing

## Scaling Limits

**Database Connection Pool:**
- Current capacity: Not explicitly configured (GORM defaults)
- Limit: Default connection pool may be insufficient for high load
- Symptoms at limit: Connection timeouts, slow queries
- Scaling path: Configure `db.DB().SetMaxOpenConns()`, `SetMaxIdleConns()`, monitor connection usage

**Webhook Processing:**
- Current capacity: Single-threaded async processing per webhook
- Limit: High webhook volume could cause processing delays
- Symptoms at limit: Webhook backlog, delayed state updates
- Scaling path: Implement worker pool, queue-based processing with multiple workers

---

*Concerns audit: 2026-01-11*
*Update as issues are fixed or new ones discovered*
