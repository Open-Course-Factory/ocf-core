package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	cors "github.com/rs/cors/wrapper/gin"

	config "soli/formations/src/configuration"
	generator "soli/formations/src/generationEngine"
	slidev "soli/formations/src/generationEngine/slidev_integration"
	"soli/formations/src/payment"
	paymentServices "soli/formations/src/payment/services"

	authController "soli/formations/src/auth"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	authHooks "soli/formations/src/auth/hooks"
	authMiddleware "soli/formations/src/auth/middleware"
	accessController "soli/formations/src/auth/routes/accessesRoutes"
	emailVerificationController "soli/formations/src/auth/routes/emailVerificationRoutes"
	// groupController "soli/formations/src/auth/routes/groupsRoutes" // Legacy Casdoor groups - replaced by class-groups
	impersonationController "soli/formations/src/auth/routes/impersonationRoutes"
	passwordResetController "soli/formations/src/auth/routes/passwordResetRoutes"
	adminUsersController "soli/formations/src/admin/routes/adminUsersRoutes"
	observabilityController "soli/formations/src/observability/routes"
userController "soli/formations/src/auth/routes/usersRoutes"
	permissionReferenceRoutes "soli/formations/src/auth/routes/permissionReferenceRoutes"
	securityAdminController "soli/formations/src/auth/routes/securityAdminRoutes"
	authServices "soli/formations/src/auth/services"
	emailController "soli/formations/src/email/routes"
	emailServices "soli/formations/src/email/services"
	"soli/formations/src/feedback"
	courseHooks "soli/formations/src/courses/hooks"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	generationController "soli/formations/src/courses/routes/generationRoutes"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	genericController "soli/formations/src/entityManagement/routes"
	swaggerGenerator "soli/formations/src/entityManagement/swagger"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentController "soli/formations/src/payment/routes"
	groupHooks "soli/formations/src/groups/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	organizationController "soli/formations/src/organizations/routes"
	terminalController "soli/formations/src/terminalTrainer/routes"
	terminalHooks "soli/formations/src/terminalTrainer/hooks"
	terminalServices "soli/formations/src/terminalTrainer/services"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	scenarioController "soli/formations/src/scenarios/routes"
	versionController "soli/formations/src/version"
	sshClientController "soli/formations/src/webSsh/routes/sshClientRoutes"

	sqldb "soli/formations/src/db"

	// Import new initialization package
	"soli/formations/src/cli"
	"soli/formations/src/cron"
	"soli/formations/src/initialization"
)

//	@title			OCF API
//	@version		0.0.1
//	@description	This is a server to build and generate courses and labs.
//	@termsOfService	TODO

//	@securityDefinitions.apikey	Bearer
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and JWT token.

func main() {
	envFile := ".env"

	err := godotenv.Load(envFile)
	if err != nil {
		log.Default().Println(err)
	}

	// Set up signal handling: SIGTERM (k8s pod stop) and SIGINT (Ctrl-C in dev).
	shutdownCtx, shutdownCancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer shutdownCancel()

	// Initialize Casdoor connection
	casdoor.InitCasdoorConnection("", envFile)

	// Initialize database connection
	sqldb.InitDBConnection(envFile)

	// Perform all database migrations
	initialization.AutoMigrateAll(sqldb.DB)

	// Initialize Casdoor enforcer
	casdoor.InitCasdoorEnforcer(sqldb.DB, "")

	// Register all entities in the entity management system
	initialization.RegisterEntities()

	// Initialize payment entities and hooks
	// NOTE: Must be before InitDevelopmentData so that SubscriptionPlan entity
	// is registered when assignFreeTrialPlan() runs during test user setup.
	//
	// The shared StripeSyncQueue is returned so the worker and the admin
	// /admin/stripe/pending-syncs endpoint can be wired on the same instance.
	stripeSyncQueue := payment.InitPaymentEntities(sqldb.DB)

	// Hydrate the size catalog from tt-backend. Best-effort: failure
	// leaves the hardcoded fallback active so ocf-core still boots.
	initialization.HydrateSizeCatalog(sqldb.DB)

	// Start the StripeSyncWorker: a single goroutine that polls the queue and
	// drains pending rows to Stripe. Survives ocf-core restarts via the queue.
	stripeSyncWorker := paymentServices.NewStripeSyncWorker(stripeSyncQueue, paymentServices.NewStripeService(sqldb.DB))
	stripeSyncWorker.Start(shutdownCtx)
	// stripeSyncWorker.Shutdown(10s) is called below in the orderly-shutdown
	// block, after http.Server.Shutdown drains in-flight requests.
	log.Println("✅ Stripe sync worker started")

	// Setup development data (test users, default subscription plans)
	initialization.InitDevelopmentData(sqldb.DB)

	// Initialize email templates
	emailServices.InitDefaultTemplates(sqldb.DB)

	// Setup role-based permissions — each module registers its own policies
	log.Println("Setting up role-based permissions...")
	casdoor.Enforcer.LoadPolicy()
	userController.RegisterAuthPermissions(casdoor.Enforcer)
	userController.RegisterUserPermissions(casdoor.Enforcer)
	userController.RegisterFeedbackPermissions(casdoor.Enforcer)
	terminalController.RegisterTerminalPermissions(casdoor.Enforcer)
	securityAdminController.RegisterSecurityAdminPermissions(casdoor.Enforcer)
	scenarioController.RegisterScenarioPermissions(casdoor.Enforcer)
	courseController.RegisterCoursePermissions(casdoor.Enforcer)
	paymentController.RegisterPaymentPermissions(casdoor.Enforcer)
	paymentController.RegisterAdminStripePermissions(casdoor.Enforcer)
	organizationController.RegisterOrganizationPermissions(casdoor.Enforcer)
	impersonationController.RegisterImpersonationPermissions(casdoor.Enforcer)
	adminUsersController.RegisterPermissions(casdoor.Enforcer)
	observabilityController.RegisterPermissions(casdoor.Enforcer)
	log.Println("✅ All permissions setup completed")

	// Register Layer 2 enforcement handlers (business logic authorization)
	entityLoader := access.NewGormEntityLoader(sqldb.DB)
	memberChecker := access.NewGormMembershipChecker(sqldb.DB)
	access.RegisterBuiltinEnforcers(entityLoader, memberChecker)
	log.Println("✅ Layer 2 enforcement handlers registered")

	// Initialize remaining hooks
	courseHooks.InitCourseHooks(sqldb.DB)
	authHooks.InitAuthHooks(sqldb.DB)
	groupHooks.InitGroupHooks(sqldb.DB)
	organizationHooks.InitOrganizationHooks(sqldb.DB) // Phase 1: Organization hooks
	terminalHooks.InitTerminalHooks(sqldb.DB)         // Terminal permission hooks
	scenarioHooks.InitScenarioHooks(sqldb.DB)         // Scenario assignment authorization hooks

	// Auto-register the write-side ownership hooks from every entity's declared
	// OwnershipConfig. Runs after all entities are registered (configs stored)
	// and the DB is ready. This replaces the per-module manual NewOwnershipHook
	// registrations that used to live in the Init*Hooks funcs above.
	ems.RegisterOwnershipHooks(sqldb.DB)

	// Register module features
	initialization.RegisterModuleFeatures(sqldb.DB)

	// Impersonation service — shared between routes, middleware, and the
	// background expiry goroutine below.
	impersonationSvc := authServices.NewImpersonationService(sqldb.DB)
	impersonationValidator := impersonationController.NewCasdoorValidatorAdapter(casdoor.NewCasdoorUserValidator())
	impersonationRoles := func(uid string) ([]string, error) {
		return casdoor.Enforcer.GetRolesForUser(uid)
	}

	// Wire impersonation into AuthManagement so the swap takes effect on every
	// authenticated route. AuthManagement is registered per-route across the
	// codebase, so this single configuration call covers all callers without
	// having to hoist AuthManagement up to the apiGroup level.
	authController.SetImpersonationHandler(authMiddleware.ImpersonationMiddleware(impersonationSvc, impersonationRoles))

	// ✅ SECURITY: Start background jobs
	cron.StartWebhookCleanupJob(sqldb.DB)
	cron.StartAuditLogCleanupJob(sqldb.DB) // Start audit log cleanup (retention management)
	cron.StartEmailVerificationCleanupJob(sqldb.DB)    // Clean up expired email verification tokens
	cron.StartScenarioSessionCleanupJob(sqldb.DB)      // Abandon zombie scenario sessions with dead terminals

	// Background job: close idle impersonation sessions every minute. Mirrors
	// the safety net described in src/auth/services/impersonationService.go.
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if n, err := impersonationSvc.ExpireStale(authServices.ImpersonationIdleTimeout); err != nil {
				log.Printf("[impersonation] ExpireStale error: %v", err)
			} else if n > 0 {
				log.Printf("[impersonation] expired %d stale session(s)", n)
			}
		}
	}()

	// Parse CLI flags for course generation
	if cli.ParseFlags(sqldb.DB, casdoor.Enforcer) {
		os.Exit(0)
	}

	// Set default slide engine
	generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}

	// Initialize Gin router
	r := gin.Default()

	// Setup CORS middleware - SECURE CONFIGURATION
	allowedOrigins := config.InitAllowedOrigins()
	environment := os.Getenv("ENVIRONMENT")

	log.Printf("CORS allowed origins: %v", allowedOrigins)

	r.Use(cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Authorization",
			"Content-Type",
			"Accept",
			"X-Requested-With",
			"Origin",
			"Access-Control-Request-Method",
			"Access-Control-Request-Headers",
			"X-Impersonate-User",
		},
		ExposedHeaders: []string{
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"Content-Length",
			"Access-Control-Allow-Origin",
		},
		MaxAge:             300,  // 5 minutes
		Debug:              environment == "development", // Enable debug in dev mode
		OptionsPassthrough: false, // Handle OPTIONS here, don't pass through
	}))

	// Setup API routes
	apiGroup := r.Group("/api/v1")

	// Layer 2 enforcement — applied globally, acts only on routes registered in RouteRegistry
	apiGroup.Use(access.Layer2Enforcement())

	// Version endpoint (no auth required)
	versionCtrl := versionController.NewVersionController()
	apiGroup.GET("/version", versionCtrl.GetVersion)

	// Register module routes
	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	authController.AuthRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	passwordResetController.PasswordResetRoutes(apiGroup.Group("/auth"), sqldb.DB) // Public password reset routes
	emailVerificationController.EmailVerificationRoutes(apiGroup.Group("/auth"), sqldb.DB) // Public email verification routes
	genericController.HooksRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

userController.UsersRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	// NOTE: Commented out legacy Casdoor group routes - replaced by new class-groups system
	// groupController.GroupRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	accessController.AccessRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sshClientController.SshClientRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	generationController.GenerationsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	terminalController.TerminalRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	terminalController.UserTerminalKeyRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	organizationController.OrganizationRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	emailController.EmailTemplateRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	securityAdminController.SecurityAdminRoutes(apiGroup, sqldb.DB)
	permissionReferenceRoutes.PermissionReferenceRoutes(apiGroup)
	scenarioController.ScenarioRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	feedback.FeedbackRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	impersonationController.ImpersonationRoutes(apiGroup, sqldb.DB, impersonationSvc, impersonationValidator)
	adminUsersController.RegisterRoutes(apiGroup, sqldb.DB)
	observabilityController.RegisterRoutes(apiGroup, sqldb.DB)

	// Initialize payment routes
	payment.InitPaymentRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	// Admin Stripe queue visibility (admin only) — shares the queue instance
	// constructed in InitPaymentEntities so the hook, worker, and endpoint all
	// see the same durable rows.
	paymentController.RegisterAdminStripeRoutes(apiGroup, sqldb.DB, stripeSyncQueue)

	// Install the plan-gating chain builder before routes are mounted, so any
	// action declaring a PlanRequirement resolves into the canonical payment
	// middlewares. Injected (not imported) to keep the swagger package free of a
	// payment import — see swaggerGenerator.SetPlanChainBuilder.
	planChainTerminalService := terminalServices.NewTerminalTrainerService(sqldb.DB)
	swaggerGenerator.SetPlanChainBuilder(func(req entityManagementInterfaces.PlanRequirement) []gin.HandlerFunc {
		return paymentMiddleware.PlanChain(sqldb.DB, req, planChainTerminalService)
	})

	// Initialize Swagger documentation
	initialization.InitSwagger(r, sqldb.DB)

	// Validate permission setup
	access.ValidatePermissionSetup(r)
	if os.Getenv("PERMISSION_VALIDATION_STRICT") == "true" {
		if err := access.ValidatePermissionSetupStrict(r); err != nil {
			log.Fatalf("strict permission validation failed: %v", err)
		}
	}

	// Run the HTTP server in a goroutine so we can wait for the shutdown signal.
	server := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		log.Printf("HTTP server starting on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
		close(serverErrCh)
	}()

	// Wait for either a fatal server error or a shutdown signal.
	select {
	case err := <-serverErrCh:
		if err != nil {
			log.Fatalf("HTTP server failed: %v", err)
		}
	case <-shutdownCtx.Done():
		log.Println("shutdown signal received — beginning orderly shutdown")
	}

	// Orderly shutdown: HTTP first (stop accepting new connections, drain
	// in-flight requests), then the Stripe sync worker. Each capped at 10s —
	// total worst-case ~20s. Recommend terminationGracePeriodSeconds: 30+ on
	// the deployment.
	const shutdownStepTimeout = 10 * time.Second

	httpShutdownCtx, httpShutdownCancel := context.WithTimeout(context.Background(), shutdownStepTimeout)
	if err := server.Shutdown(httpShutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	httpShutdownCancel()

	stripeSyncWorker.Shutdown(shutdownStepTimeout)

	log.Println("shutdown complete")
}
