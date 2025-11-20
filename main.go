package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	cors "github.com/rs/cors/wrapper/gin"

	config "soli/formations/src/configuration"
	generator "soli/formations/src/generationEngine"
	slidev "soli/formations/src/generationEngine/slidev_integration"
	"soli/formations/src/payment"

	authController "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"
	authHooks "soli/formations/src/auth/hooks"
	accessController "soli/formations/src/auth/routes/accessesRoutes"
	// groupController "soli/formations/src/auth/routes/groupsRoutes" // Legacy Casdoor groups - replaced by class-groups
	passwordResetController "soli/formations/src/auth/routes/passwordResetRoutes"
	sshKeyController "soli/formations/src/auth/routes/sshKeysRoutes"
	userController "soli/formations/src/auth/routes/usersRoutes"
	emailController "soli/formations/src/email/routes"
	emailServices "soli/formations/src/email/services"
	courseHooks "soli/formations/src/courses/hooks"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	generationController "soli/formations/src/courses/routes/generationRoutes"
	genericController "soli/formations/src/entityManagement/routes"
	groupHooks "soli/formations/src/groups/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	organizationController "soli/formations/src/organizations/routes"
	terminalController "soli/formations/src/terminalTrainer/routes"
	versionController "soli/formations/src/version"
	sshClientController "soli/formations/src/webSsh/routes/sshClientRoutes"

	sqldb "soli/formations/src/db"

	paymentMiddleware "soli/formations/src/payment/middleware"

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

	// Setup development data (test users, default subscription plans)
	initialization.InitDevelopmentData(sqldb.DB)

	// Initialize email templates
	emailServices.InitDefaultTemplates(sqldb.DB)

	// Setup payment role permissions
	initialization.SetupPaymentRolePermissions(casdoor.Enforcer)

	// Initialize payment entities and hooks
	payment.InitPaymentEntities(sqldb.DB)
	courseHooks.InitCourseHooks(sqldb.DB)
	authHooks.InitAuthHooks(sqldb.DB)
	groupHooks.InitGroupHooks(sqldb.DB)
	organizationHooks.InitOrganizationHooks(sqldb.DB) // Phase 1: Organization hooks

	// Register module features
	initialization.RegisterModuleFeatures(sqldb.DB)

	// ✅ SECURITY: Start background jobs
	cron.StartWebhookCleanupJob(sqldb.DB)
	cron.StartAuditLogCleanupJob(sqldb.DB) // Start audit log cleanup (retention management)

	// Parse CLI flags for course generation
	if cli.ParseFlags(sqldb.DB, casdoor.Enforcer) {
		os.Exit(0)
	}

	// Set default slide engine
	generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}

	// Initialize Gin router
	r := gin.Default()

	// Setup CORS middleware - SECURE CONFIGURATION
	// Get allowed origins from environment variables
	allowedOrigins := []string{}
	environment := os.Getenv("ENVIRONMENT")

	if frontendURL := os.Getenv("FRONTEND_URL"); frontendURL != "" {
		allowedOrigins = append(allowedOrigins, frontendURL)
	}
	if adminURL := os.Getenv("ADMIN_FRONTEND_URL"); adminURL != "" {
		allowedOrigins = append(allowedOrigins, adminURL)
	}

	// For local development, add common localhost ports
	if environment == "development" || environment == "" || len(allowedOrigins) == 0 {
		log.Println("🔓 Development mode: CORS allowing common localhost origins")
		allowedOrigins = append(allowedOrigins,
			"http://localhost:3000",   // React default
			"http://localhost:3001",   // React alternative
			"http://localhost:4000",   // Custom frontend port
			"http://localhost:5173",   // Vite default
			"http://localhost:5174",   // Vite alternative
			"http://localhost:8080",   // Backend
			"http://localhost:8081",   // Alternative backend
			"http://127.0.0.1:3000",   // Explicit 127.0.0.1
			"http://127.0.0.1:4000",
			"http://127.0.0.1:5173",
			"http://127.0.0.1:8080",
		)
	}

	log.Printf("🔒 CORS allowed origins: %v", allowedOrigins)

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

	// Setup payment middlewares
	usageLimitMiddleware := paymentMiddleware.NewUsageLimitMiddleware(sqldb.DB)
	// Note: userRoleMiddleware removed - subscription role updates should be done
	// in specific route handlers AFTER authentication, not globally

	// Setup API routes
	apiGroup := r.Group("/api/v1")

	// Version endpoint (no auth required)
	versionCtrl := versionController.NewVersionController()
	apiGroup.GET("/version", versionCtrl.GetVersion)

	// Register module routes
	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	authController.AuthRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	passwordResetController.PasswordResetRoutes(apiGroup.Group("/auth"), sqldb.DB) // Public password reset routes
	genericController.HooksRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	sshKeyController.SshKeysRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	userController.UsersRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	// NOTE: Commented out legacy Casdoor group routes - replaced by new class-groups system
	// groupController.GroupRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	accessController.AccessRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sshClientController.SshClientRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	generationController.GenerationsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	terminalController.TerminalRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	terminalController.UserTerminalKeyRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	organizationController.OrganizationRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	organizationController.OrganizationMigrationRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	emailController.EmailTemplateRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	// Setup usage limit middleware for specific routes
	apiGroupWithUsageCheck := apiGroup.Group("")
	apiGroupWithUsageCheck.Use(usageLimitMiddleware.CheckUsageForPath())

	// Initialize payment routes
	payment.InitPaymentRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	// Initialize Swagger documentation
	initialization.InitSwagger(r, sqldb.DB)

	// Start server
	r.Run(":8080")
}
