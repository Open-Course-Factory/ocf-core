package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	cors "github.com/rs/cors/wrapper/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"soli/formations/docs"

	config "soli/formations/src/configuration"
	generator "soli/formations/src/generationEngine"
	marp "soli/formations/src/generationEngine/marp_integration"
	slidev "soli/formations/src/generationEngine/slidev_integration"
	"soli/formations/src/payment"
	testtools "soli/formations/tests/testTools"

	authController "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"
	authRegistration "soli/formations/src/auth/entityRegistration"
	authModels "soli/formations/src/auth/models"
	accessController "soli/formations/src/auth/routes/accessesRoutes"
	groupController "soli/formations/src/auth/routes/groupsRoutes"
	sshKeyController "soli/formations/src/auth/routes/sshKeysRoutes"
	userController "soli/formations/src/auth/routes/usersRoutes"
	courseRegistration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"
	courseModels "soli/formations/src/courses/models"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	generationController "soli/formations/src/courses/routes/generationRoutes"
	genericController "soli/formations/src/entityManagement/routes"
	labRegistration "soli/formations/src/labs/entityRegistration"
	labModels "soli/formations/src/labs/models"
	connectionController "soli/formations/src/labs/routes/connectionRoutes"
	machineController "soli/formations/src/labs/routes/machineRoutes"
	usernameController "soli/formations/src/labs/routes/usernameRoutes"
	terminalRegistration "soli/formations/src/terminalTrainer/entityRegistration"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	versionController "soli/formations/src/version"
	sshClientController "soli/formations/src/webSsh/routes/sshClientRoutes"

	courseDto "soli/formations/src/courses/dto"
	courseService "soli/formations/src/courses/services"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	genericService "soli/formations/src/entityManagement/services"

	ems "soli/formations/src/entityManagement/entityManagementService"

	sqldb "soli/formations/src/db"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	swaggerGenerator "soli/formations/src/entityManagement/swagger"

	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
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

	casdoor.InitCasdoorConnection("", envFile)

	sqldb.InitDBConnection(envFile)

	sqldb.DB.AutoMigrate()

	sqldb.DB.AutoMigrate(&courseModels.Page{})
	sqldb.DB.AutoMigrate(&courseModels.Section{})
	sqldb.DB.AutoMigrate(&courseModels.Chapter{})
	sqldb.DB.AutoMigrate(&courseModels.Course{})
	sqldb.DB.AutoMigrate(&courseModels.Session{})

	sqldb.DB.AutoMigrate(&courseModels.CourseChapters{})
	errJTChapterC := sqldb.DB.SetupJoinTable(&courseModels.Course{}, "Chapters", &courseModels.CourseChapters{})
	if errJTChapterC != nil {
		log.Default().Println(errJTChapterC)
	}
	errJTCoursesC := sqldb.DB.SetupJoinTable(&courseModels.Chapter{}, "Courses", &courseModels.CourseChapters{})
	if errJTCoursesC != nil {
		log.Default().Println(errJTCoursesC)
	}

	sqldb.DB.AutoMigrate(&courseModels.ChapterSections{})
	errJTSectionC := sqldb.DB.SetupJoinTable(&courseModels.Chapter{}, "Sections", &courseModels.ChapterSections{})
	if errJTSectionC != nil {
		log.Default().Println(errJTSectionC)
	}
	errJTChaptersS := sqldb.DB.SetupJoinTable(&courseModels.Section{}, "Chapters", &courseModels.ChapterSections{})
	if errJTChaptersS != nil {
		log.Default().Println(errJTChaptersS)
	}

	sqldb.DB.AutoMigrate(&courseModels.SectionPages{})
	errJTPage := sqldb.DB.SetupJoinTable(&courseModels.Section{}, "Pages", &courseModels.SectionPages{})
	if errJTPage != nil {
		log.Default().Println(errJTPage)
	}
	errJTSectionP := sqldb.DB.SetupJoinTable(&courseModels.Page{}, "Sections", &courseModels.SectionPages{})
	if errJTSectionP != nil {
		log.Default().Println(errJTSectionP)
	}

	sqldb.DB.AutoMigrate(&courseModels.Schedule{})
	sqldb.DB.AutoMigrate(&courseModels.Theme{})

	sqldb.DB.AutoMigrate(&courseModels.Generation{})

	sqldb.DB.AutoMigrate(&authModels.Sshkey{})

	sqldb.DB.AutoMigrate(&labModels.Username{})
	sqldb.DB.AutoMigrate(&labModels.Machine{})
	sqldb.DB.AutoMigrate(&labModels.Connection{})

	sqldb.DB.AutoMigrate(&terminalModels.Terminal{})
	sqldb.DB.AutoMigrate(&terminalModels.UserTerminalKey{})
	sqldb.DB.AutoMigrate(&terminalModels.TerminalShare{})

	sqldb.DB.AutoMigrate(&paymentModels.SubscriptionPlan{})
	sqldb.DB.AutoMigrate(&paymentModels.UserSubscription{})
	sqldb.DB.AutoMigrate(&paymentModels.Invoice{})
	sqldb.DB.AutoMigrate(&paymentModels.PaymentMethod{})
	sqldb.DB.AutoMigrate(&paymentModels.UsageMetrics{})
	sqldb.DB.AutoMigrate(&paymentModels.BillingAddress{})

	casdoor.InitCasdoorEnforcer(sqldb.DB, "")

	ems.GlobalEntityRegistrationService.RegisterEntity(authRegistration.SshkeyRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.SessionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.CourseRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.PageRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.SectionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ChapterRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ScheduleRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ThemeRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.GenerationRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(labRegistration.MachineRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(labRegistration.ConnectionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(labRegistration.UsernameRegistration{})

	ems.GlobalEntityRegistrationService.RegisterEntity(terminalRegistration.TerminalRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(terminalRegistration.UserTerminalKeyRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(terminalRegistration.TerminalShareRegistration{})

	initDB()

	setupPaymentRolePermissions()

	payment.InitPaymentEntities(sqldb.DB)

	if parseFlags() {
		os.Exit(0)
	}

	// should be with an option to choose between slidev and marp
	generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}

	r := gin.Default()
	// r.Use(middleware.CORS())
	r.Use(cors.New(cors.Options{
		AllowedOrigins:     []string{"*"},
		AllowCredentials:   true,
		Debug:              true,
		AllowedMethods:     []string{"GET", "POST", "PATCH", "OPTIONS", "DELETE"},
		AllowedHeaders:     []string{"*"},
		OptionsPassthrough: true,
	}))

	// Middlewares globaux pour le système de paiement
	usageLimitMiddleware := paymentMiddleware.NewUsageLimitMiddleware(sqldb.DB)
	userRoleMiddleware := paymentMiddleware.NewUserRoleMiddleware(sqldb.DB)

	// Appliquer le middleware de mise à jour des rôles globalement
	r.Use(userRoleMiddleware.EnsureSubscriptionRole())

	apiGroup := r.Group("/api/v1")

	// Version endpoint (no auth required)
	versionCtrl := versionController.NewVersionController()
	apiGroup.GET("/version", versionCtrl.GetVersion)

	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	authController.AuthRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	genericController.HooksRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	sshKeyController.SshKeysRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	userController.UsersRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	groupController.GroupRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	accessController.AccessRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sshClientController.SshClientRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	machineController.MachinesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	usernameController.UsernamesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	connectionController.ConnectionsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	generationController.GenerationsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	terminalController.TerminalRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	terminalController.UserTerminalKeyRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	apiGroupWithUsageCheck := apiGroup.Group("")
	apiGroupWithUsageCheck.Use(usageLimitMiddleware.CheckUsageForPath())

	payment.InitPaymentRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	initSwagger(r)

	r.Run(":8080")
}

func initSwagger(r *gin.Engine) {
	env := os.Getenv("ENVIRONMENT")
	if env == "development" || env == "test" {
		docs.SwaggerInfo.Schemes = []string{"http", "https"}
	} else {
		docs.SwaggerInfo.Schemes = []string{"https"}
	}

	docs.SwaggerInfo.Title = "OCF API"
	docs.SwaggerInfo.Description = "This is an API to build and generate courses with labs"
	docs.SwaggerInfo.Version = os.Getenv("OCF_VERSION")

	// Setup de la documentation complète (manual + auto-generated)
	setupCompleteSwaggerSystem(r)
}

func initDB() {

	env := os.Getenv("ENVIRONMENT")
	if env == "development" || env == "test" {
		sqldb.DB = sqldb.DB.Debug()

		setupExternalUsersData()
		setupDefaultSubscriptionPlans()
	}
}

func setupExternalUsersData() {
	//testtools.DeleteAllObjects()
	users, _ := casdoorsdk.GetUsers()
	var notDeletedUser []*casdoorsdk.User
	for _, user := range users {
		if !user.IsDeleted {
			notDeletedUser = append(notDeletedUser, user)
		}
	}
	if len(notDeletedUser) == 0 {
		testtools.SetupBasicRoles()
		testtools.SetupUsers()
		testtools.SetupGroups()
		testtools.SetupRoles()
	}
}

func parseFlags() bool {

	const COURSE_FLAG = "c"
	const GIT_COURSE_REPO_FLAG = "course-repo"
	const GIT_COURSE_REPO_BRANCH_FLAG = "course-repo-branch"
	const THEME_FLAG = "t"
	const GIT_THEME_REPO_FLAG = "theme-repo"
	const GIT_THEME_REPO_BRANCH_FLAG = "theme-repo-branch"
	const TYPE_FLAG = "e"
	const DRY_RUN_FLAG = "dry-run"
	const SLIDE_ENGINE_FLAG = "slide-engine"

	courseName := flag.String(COURSE_FLAG, "git", "name of the course you need to generate")
	courseGitRepository := flag.String(GIT_COURSE_REPO_FLAG, "", "git repository")
	courseBranchGitRepository := flag.String(GIT_COURSE_REPO_BRANCH_FLAG, "main", "ssh git repository branch for course")
	courseThemeName := flag.String(THEME_FLAG, "sdv", "name of the theme used to generate the website")
	courseThemeGitRepository := flag.String(GIT_THEME_REPO_FLAG, "", "theme git repository")
	courseThemeBranchGitRepository := flag.String(GIT_THEME_REPO_BRANCH_FLAG, "main", "ssh git repository branch for theme")
	courseType := flag.String(TYPE_FLAG, "html", "type generated : html (default) or pdf")
	config.DRY_RUN = flag.Bool(DRY_RUN_FLAG, false, "if set true, the cli stops before calling slide generator")
	slideEngine := flag.String(SLIDE_ENGINE_FLAG, "slidev", "slide generator used, marp or slidev (default)")
	flag.Parse()

	fmt.Println(courseType)

	// check mandatory flags
	if !isFlagPassed(COURSE_FLAG) || !isFlagPassed(THEME_FLAG) || !isFlagPassed(TYPE_FLAG) {
		return false
	}

	switch *slideEngine {
	case "marp":
		generator.SLIDE_ENGINE = marp.MarpCourseGenerator{}
	case "slidev":
		generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}
	default:
		generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}
	}

	courseService := courseService.NewCourseService(sqldb.DB)
	course := courseService.GetCourseFromProgramInputs(courseName, courseGitRepository, courseBranchGitRepository)

	setCourseThemeFromProgramInputs(&course, courseThemeName, courseThemeGitRepository, courseThemeBranchGitRepository)

	//courseModels.FillCourseModelFromFiles(*courseName, &course)

	genericService := genericService.NewGenericService(sqldb.DB, casdoor.Enforcer)

	courseInputDto := courseDto.CourseModelToCourseInputDto(course)
	_, errorSaving := genericService.CreateEntity(courseInputDto, reflect.TypeOf(models.Course{}).Name())

	if errorSaving != nil {
		fmt.Println(errorSaving.Error())
		return true
	}

	return true
}

func setCourseThemeFromProgramInputs(course *courseModels.Course, themeName *string, themeGitRepository *string, themeGitRepositoryBranch *string) {
	course.Theme.Name = *themeName
	course.Theme.Repository = *themeGitRepository
	course.Theme.RepositoryBranch = *themeGitRepositoryBranch
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// Fonction utilitaire pour initialiser les plans d'abonnement par défaut
func setupDefaultSubscriptionPlans() {
	db := sqldb.DB

	// Vérifier si les plans existent déjà
	var count int64
	db.Model(&paymentModels.SubscriptionPlan{}).Count(&count)
	if count > 0 {
		return // Plans déjà créés
	}

	// Plan Member Pro
	memberProPlan := &paymentModels.SubscriptionPlan{
		Name:               "Member Pro",
		Description:        "Accès à un terminal",
		PriceAmount:        490, // 4.90€
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          14,
		Features:           []string{"unlimited_courses", "advanced_labs", "export", "custom_themes"},
		MaxConcurrentUsers: 1,
		MaxCourses:         0,
		MaxLabSessions:     1,
		IsActive:           true,
		RequiredRole:       "member_pro",
	}

	plans := []*paymentModels.SubscriptionPlan{memberProPlan}

	for _, plan := range plans {
		if err := db.Create(plan).Error; err != nil {
			log.Printf("Warning: Failed to create subscription plan %s: %v\n", plan.Name, err)
		} else {
			log.Printf("Created subscription plan: %s\n", plan.Name)
		}
	}
}

func setupPaymentRolePermissions() {
	casdoor.Enforcer.LoadPolicy()

	// Permissions pour Student Premium
	casdoor.Enforcer.AddPolicy("member_pro", "/api/v1/terminals/*", "(GET|POST)")
	casdoor.Enforcer.AddPolicy("member_pro", "/api/v1/subscriptions/current", "GET")
	casdoor.Enforcer.AddPolicy("member_pro", "/api/v1/subscriptions/portal", "POST")
	casdoor.Enforcer.AddPolicy("member_pro", "/api/v1/invoices/user", "GET")
	casdoor.Enforcer.AddPolicy("member_pro", "/api/v1/payment-methods/user", "GET")

	// Permissions pour Organization (hérite de supervisor_pro)
	casdoor.Enforcer.AddPolicy("organization", "/api/v1/*", "(GET|POST|PATCH|DELETE)")
	casdoor.Enforcer.AddPolicy("organization", "/api/v1/users/*", "(GET|POST|PATCH)")
	casdoor.Enforcer.AddPolicy("organization", "/api/v1/groups/*", "(GET|POST|PATCH|DELETE)")

	// Groupements de rôles (hiérarchie)
	casdoor.Enforcer.AddGroupingPolicy("member_pro", "member")
	casdoor.Enforcer.AddGroupingPolicy("organization", "member_pro")
}

func setupCompleteSwaggerSystem(r *gin.Engine) {
	log.Println("🚀 Setting up complete Swagger documentation system...")

	// Middleware d'authentification pour les routes documentées
	authMiddleware := authController.NewAuthMiddleware(sqldb.DB)

	// 📋 ÉTAPE 1: Setup des routes auto-documentées
	log.Println("  📋 Setting up auto-documented routes...")
	routeGenerator := swaggerGenerator.NewSwaggerRouteGenerator(sqldb.DB)
	docGroup := r.Group("/api/v1")
	routeGenerator.RegisterDocumentedRoutes(docGroup, authMiddleware.AuthManagement())

	// 🔀 ÉTAPE 2: Setup du merger Swagger
	log.Println("  🔀 Setting up Swagger spec merger...")
	merger := swaggerGenerator.NewSwaggerSpecMerger()

	// 📄 ÉTAPE 3: Endpoints de documentation
	setupSwaggerEndpoints(r, merger)

	// 📊 ÉTAPE 4: Endpoints de debug et statistiques
	setupDocumentationDebugEndpoints(r)

	log.Println("✅ Complete Swagger system ready!")
	log.Println("📚 Available endpoints:")
	log.Println("  🎨 /swagger/ - Complete documentation (manual + auto)")
	log.Println("  📋 /api/v1/swagger/spec - Merged OpenAPI spec")
	log.Println("  🔍 /api/v1/swagger/debug - Debug merge process")
	log.Println("  📊 /api/v1/swagger/stats - Documentation statistics")
	log.Println("  📄 /swagger/index.html - Original swag documentation")
}

func setupSwaggerEndpoints(r *gin.Engine, merger *swaggerGenerator.SwaggerSpecMerger) {
	// Endpoint principal : spec mergée
	r.GET("/api/v1/swagger/spec", func(ctx *gin.Context) {
		mergedSpec := merger.MergeSpecs()

		// Headers CORS pour Swagger UI
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Methods", "GET")
		ctx.Header("Access-Control-Allow-Headers", "Content-Type")

		ctx.JSON(200, mergedSpec)
	})

	// Interface Swagger UI personnalisée (documentation complète)
	r.GET("/swagger/", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "text/html")
		ctx.String(200, generateCustomSwaggerHTML())
	})

	// Garder l'endpoint original pour compatibilité
	r.GET("/swagger/previous", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

func setupDocumentationDebugEndpoints(r *gin.Engine) {
	// Debug: comparer les sources de documentation
	r.GET("/api/v1/swagger/debug", func(ctx *gin.Context) {
		autoSpec := swaggerGenerator.NewDocumentationGenerator().GenerateOpenAPISpec()
		swaggerConfigs := ems.GlobalEntityRegistrationService.GetAllSwaggerConfigs()

		debugInfo := map[string]interface{}{
			"auto_generated_spec":  autoSpec,
			"documented_entities":  len(swaggerConfigs),
			"entity_details":       swaggerConfigs,
			"generation_timestamp": time.Now().Format(time.RFC3339),
			"merge_strategy":       "manual_priority_over_auto",
		}

		ctx.JSON(200, debugInfo)
	})

	// Statistiques de documentation
	r.GET("/api/v1/swagger/stats", func(ctx *gin.Context) {
		swaggerConfigs := ems.GlobalEntityRegistrationService.GetAllSwaggerConfigs()

		stats := map[string]interface{}{
			"total_documented_entities": len(swaggerConfigs),
			"entities_with_swagger":     getEntitiesWithSwagger(swaggerConfigs),
			"auto_generated_routes":     countAutoGeneratedRoutes(swaggerConfigs),
			"documentation_coverage":    calculateDocumentationCoverage(swaggerConfigs),
			"generation_time":           time.Now().Format(time.RFC3339),
		}

		ctx.JSON(200, stats)
	})
}

// generateCustomSwaggerHTML génère une page HTML Swagger UI personnalisée
func generateCustomSwaggerHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>OCF API Documentation - Complete</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui.css" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
        
        .header-banner {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 15px;
            text-align: center;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        
        .header-banner h1 {
            margin: 0;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
        }
        
        .header-banner p {
            margin: 5px 0 0 0;
            opacity: 0.9;
            font-size: 14px;
        }
        
        .status-badge {
            display: inline-block;
            background: #28a745;
            color: white;
            padding: 4px 8px;
            border-radius: 12px;
            font-size: 12px;
            margin-left: 10px;
        }
        
        /* Style pour les entités auto-générées */
        .swagger-ui .opblock.opblock-get .opblock-summary-control,
        .swagger-ui .opblock.opblock-post .opblock-summary-control,
        .swagger-ui .opblock.opblock-patch .opblock-summary-control,
        .swagger-ui .opblock.opblock-delete .opblock-summary-control {
            position: relative;
        }
    </style>
</head>
<body>
    <div class="header-banner">
        <h1>🚀 OCF API Documentation</h1>
        <p>Documentation complète : Endpoints manuels + Entités auto-générées <span class="status-badge">🤖 Hybrid</span></p>
    </div>
    
    <div id="swagger-ui"></div>
    
    <script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: window.location.origin + '/api/v1/swagger/spec',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                defaultModelsExpandDepth: 1,
                defaultModelExpandDepth: 1,
                docExpansion: "list",
                tagsSorter: "alpha",
                operationsSorter: "alpha",
                filter: true,
                validatorUrl: null,
                tryItOutEnabled: true,
                onComplete: function() {
                    console.log('📚 OCF API Documentation chargée');
                    console.log('🔀 Documentation hybride : manuelle + auto-générée');

										// 🔍 DEBUG : Vérifier les serveurs configurés
                    const spec = ui.getSystem().specSelectors.spec().toJS();
                    console.log('🔍 Servers in spec:', spec.servers);
                    
                    // Ajouter un indicateur de statut dans le header
                    setTimeout(() => {
                        const infoSection = document.querySelector('.information-container');
                        if (infoSection) {
                            const statusDiv = document.createElement('div');
                            statusDiv.style.cssText = 'background: #e8f5e8; padding: 10px; border-radius: 5px; margin: 10px 0; border-left: 4px solid #28a745;';
                            statusDiv.innerHTML = '<strong>🔄 Documentation dynamique :</strong> Cette documentation est générée automatiquement et reste toujours synchronisée avec le code.';
                            infoSection.appendChild(statusDiv);
                        }
                    }, 1000);
                }
            });
        };
    </script>
</body>
</html>`
}

func getEntitiesWithSwagger(configs map[string]*entityManagementInterfaces.EntitySwaggerConfig) []string {
	var entities []string
	for entityName := range configs {
		entities = append(entities, entityName)
	}
	return entities
}

func countAutoGeneratedRoutes(configs map[string]*entityManagementInterfaces.EntitySwaggerConfig) int {
	count := 0
	for _, config := range configs {
		if config.GetAll != nil {
			count++
		}
		if config.GetOne != nil {
			count++
		}
		if config.Create != nil {
			count++
		}
		if config.Update != nil {
			count++
		}
		if config.Delete != nil {
			count++
		}
	}
	return count
}

func calculateDocumentationCoverage(configs map[string]*entityManagementInterfaces.EntitySwaggerConfig) map[string]interface{} {
	totalConfigs := len(configs)

	coverage := map[string]interface{}{
		"total_entities":      totalConfigs,
		"coverage_percentage": 100.0, // Toutes les entités enregistrées sont documentées
		"breakdown": map[string]int{
			"get_all_implemented": 0,
			"get_one_implemented": 0,
			"create_implemented":  0,
			"update_implemented":  0,
			"delete_implemented":  0,
		},
	}

	breakdown := coverage["breakdown"].(map[string]int)
	for _, config := range configs {
		if config.GetAll != nil {
			breakdown["get_all_implemented"]++
		}
		if config.GetOne != nil {
			breakdown["get_one_implemented"]++
		}
		if config.Create != nil {
			breakdown["create_implemented"]++
		}
		if config.Update != nil {
			breakdown["update_implemented"]++
		}
		if config.Delete != nil {
			breakdown["delete_implemented"]++
		}
	}

	return coverage
}
