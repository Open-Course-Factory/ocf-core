package main

import (
	"flag"
	"fmt"
	"log"
	"os"

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
	courseModels "soli/formations/src/courses/models"
	chapterController "soli/formations/src/courses/routes/chapterRoutes"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	generationController "soli/formations/src/courses/routes/generationRoutes"
	pageController "soli/formations/src/courses/routes/pageRoutes"
	scheduleController "soli/formations/src/courses/routes/scheduleRoutes"
	sectionController "soli/formations/src/courses/routes/sectionRoutes"
	sessionController "soli/formations/src/courses/routes/sessionRoutes"
	themeController "soli/formations/src/courses/routes/themeRoutes"
	labRegistration "soli/formations/src/labs/entityRegistration"
	labModels "soli/formations/src/labs/models"
	connectionController "soli/formations/src/labs/routes/connectionRoutes"
	machineController "soli/formations/src/labs/routes/machineRoutes"
	usernameController "soli/formations/src/labs/routes/usernameRoutes"
	terminalRegistration "soli/formations/src/terminalTrainer/entityRegistration"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	sshClientController "soli/formations/src/webSsh/routes/sshClientRoutes"

	courseDto "soli/formations/src/courses/dto"
	courseService "soli/formations/src/courses/services"
	genericService "soli/formations/src/entityManagement/services"

	ems "soli/formations/src/entityManagement/entityManagementService"

	sqldb "soli/formations/src/db"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	paymentRegistration "soli/formations/src/payment/entityRegistration"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"
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

	sqldb.DB.AutoMigrate(&paymentModels.SubscriptionPlan{})
	sqldb.DB.AutoMigrate(&paymentModels.UserSubscription{})
	sqldb.DB.AutoMigrate(&paymentModels.Invoice{})
	sqldb.DB.AutoMigrate(&paymentModels.PaymentMethod{})
	sqldb.DB.AutoMigrate(&paymentModels.UsageMetrics{})
	sqldb.DB.AutoMigrate(&paymentModels.BillingAddress{})

	errJTSubscription := sqldb.DB.SetupJoinTable(&paymentModels.UserSubscription{}, "SubscriptionPlan", &paymentModels.SubscriptionPlan{})
	if errJTSubscription != nil {
		log.Default().Println(errJTSubscription)
	}

	// Setup des relations pour Invoice -> UserSubscription
	errJTInvoice := sqldb.DB.SetupJoinTable(&paymentModels.Invoice{}, "UserSubscription", &paymentModels.UserSubscription{})
	if errJTInvoice != nil {
		log.Default().Println(errJTInvoice)
	}

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

	ems.GlobalEntityRegistrationService.RegisterEntity(paymentRegistration.SubscriptionPlanRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(paymentRegistration.UserSubscriptionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(paymentRegistration.PaymentMethodRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(paymentRegistration.InvoiceRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(paymentRegistration.BillingAddressRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(paymentRegistration.UsageMetricsRegistration{})

	initDB()

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
	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	scheduleController.SchedulesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	themeController.ThemesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	pageController.PagesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sectionController.SectionsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	chapterController.ChaptersRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sessionController.SessionsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	authController.AuthRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
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

	paymentController.SubscriptionRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	paymentController.PaymentMethodRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	paymentController.InvoiceRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	paymentController.BillingAddressRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	paymentController.UsageMetricsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	paymentController.WebhookRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

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
	docs.SwaggerInfo.Host = os.Getenv("OCF_API_URL")
	docs.SwaggerInfo.BasePath = "/api/v1"
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
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

	genericService := genericService.NewGenericService(sqldb.DB)

	courseInputDto := courseDto.CourseModelToCourseInputDto(course)
	_, errorSaving := genericService.CreateEntity(courseInputDto, "Course")

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
		PriceAmount:        490, // 9.90€
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          14,
		Features:           `{"unlimited_courses": false, "advanced_labs": false, "export": false, "custom_themes": false}`,
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
