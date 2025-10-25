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
	sshKeyController "soli/formations/src/auth/routes/sshKeysRoutes"
	userController "soli/formations/src/auth/routes/usersRoutes"
	courseHooks "soli/formations/src/courses/hooks"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	generationController "soli/formations/src/courses/routes/generationRoutes"
	genericController "soli/formations/src/entityManagement/routes"
	groupHooks "soli/formations/src/groups/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	terminalController "soli/formations/src/terminalTrainer/routes"
	versionController "soli/formations/src/version"
	sshClientController "soli/formations/src/webSsh/routes/sshClientRoutes"

	sqldb "soli/formations/src/db"

	paymentMiddleware "soli/formations/src/payment/middleware"

	// Import new initialization package
	"soli/formations/src/cli"
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

	// Parse CLI flags for course generation
	if cli.ParseFlags(sqldb.DB, casdoor.Enforcer) {
		os.Exit(0)
	}

	// Set default slide engine
	generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}

	// Initialize Gin router
	r := gin.Default()

	// Setup CORS middleware
	r.Use(cors.New(cors.Options{
		AllowedOrigins:     []string{"*"},
		AllowCredentials:   true,
		Debug:              true,
		AllowedMethods:     []string{"GET", "POST", "PUT", "PATCH", "OPTIONS", "DELETE"},
		AllowedHeaders:     []string{"*"},
		OptionsPassthrough: true,
	}))

	// Setup payment middlewares
	usageLimitMiddleware := paymentMiddleware.NewUsageLimitMiddleware(sqldb.DB)
	userRoleMiddleware := paymentMiddleware.NewUserRoleMiddleware(sqldb.DB)

	// Apply subscription role middleware globally
	r.Use(userRoleMiddleware.EnsureSubscriptionRole())

	// Setup API routes
	apiGroup := r.Group("/api/v1")

	// Version endpoint (no auth required)
	versionCtrl := versionController.NewVersionController()
	apiGroup.GET("/version", versionCtrl.GetVersion)

	// Register module routes
	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	authController.AuthRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
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
