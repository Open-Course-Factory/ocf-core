package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	cors "github.com/rs/cors/wrapper/gin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"soli/formations/docs"

	config "soli/formations/src/configuration"
	generator "soli/formations/src/generationEngine"
	marp "soli/formations/src/generationEngine/marp_integration"
	slidev "soli/formations/src/generationEngine/slidev_integration"
	testtools "soli/formations/src/testTools"

	authController "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"
	authDtos "soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	sshKeyController "soli/formations/src/auth/routes/sshKeysRoutes"
	userController "soli/formations/src/auth/routes/usersRoutes"
	courseModels "soli/formations/src/courses/models"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	sessionController "soli/formations/src/courses/routes/sessionRoutes"

	courseService "soli/formations/src/courses/services"

	ems "soli/formations/src/entityManagement/entityManagementService"

	sqldb "soli/formations/src/db"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// @title OCF API
// @version 0.0.1
// @description This is a server to build and generate slides.
// @termsOfService TODO

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @contact.name Solution Libre
// @contact.url https://www.solution-libre.fr
// @contact.email contact@solution-libre.fr
// @host localhost:8080
// @BasePath /api/v1
func main() {

	// ems.GlobalEntityRegistrationService.RegisterEntityInterface(reflect.TypeOf(courseModels.Course{}).Name(), courseModels.Course{})
	// ems.GlobalEntityRegistrationService.RegisterEntityConversionFunctions(reflect.TypeOf(courseModels.Course{}).Name(), courseDtos.CourseModelToCourseOutputDto)
	// var coursesDtos []interface{}
	// coursesDtos = append(coursesDtos, courseDtos.CreateCourseInput{}, courseDtos.CreateCourseOutput{})
	// ems.GlobalEntityRegistrationService.RegisterEntityDtos(reflect.TypeOf(courseModels.Course{}).Name(), coursesDtos)

	// ems.GlobalEntityRegistrationService.RegisterEntityInterface(reflect.TypeOf(courseModels.Session{}).Name(), courseModels.Session{})
	// ems.GlobalEntityRegistrationService.RegisterEntityConversionFunctions(reflect.TypeOf(courseModels.Session{}).Name(), courseDtos.SessionModelToSessionOutputDto, courseDtos.SessionInputDtoToSessionModel)

	// sessionsDtos := make(map[ems.DtoWay]interface{})
	// sessionsDtos[ems.InputDto] = courseDtos.CreateSessionInput{}
	// sessionsDtos[ems.OutputDto] = courseDtos.CreateSessionOutput{}

	// ems.GlobalEntityRegistrationService.RegisterEntityDtos(reflect.TypeOf(courseModels.Session{}).Name(), sessionsDtos)

	ems.GlobalEntityRegistrationService.RegisterEntityInterface(reflect.TypeOf(authModels.Sshkey{}).Name(), authModels.Sshkey{})
	// ems.GlobalEntityRegistrationService.RegisterEntityConversionFunctions(reflect.TypeOf(authModels.Sshkey{}).Name(), authDtos.SshkeyPtrModelToSshkeyOutput, authDtos.SshkeyModelToSshkeyOutput, authDtos.SshkeyInputDtoToSshkeyModel)
	ems.GlobalEntityRegistrationService.RegisterEntityConversionFunctions(reflect.TypeOf(authModels.Sshkey{}).Name(), authDtos.SshkeyModelToSshkeyOutput, authDtos.SshkeyInputDtoToSshkeyModel)
	sshkeyDtos := make(map[ems.DtoWay]interface{})
	sshkeyDtos[ems.InputDto] = authDtos.CreateSshkeyInput{}
	sshkeyDtos[ems.OutputDto] = authDtos.CreateSshkeyOutput{}
	ems.GlobalEntityRegistrationService.RegisterEntityDtos(reflect.TypeOf(authModels.Sshkey{}).Name(), sshkeyDtos)

	casdoor.InitCasdoorConnection()

	sqldb.InitDBConnection()

	sqldb.DB.AutoMigrate()

	sqldb.DB.AutoMigrate(&courseModels.Page{})
	sqldb.DB.AutoMigrate(&courseModels.Section{})
	sqldb.DB.AutoMigrate(&courseModels.Chapter{})
	sqldb.DB.AutoMigrate(&courseModels.Course{})

	sqldb.DB.AutoMigrate(&authModels.Sshkey{})

	casdoor.InitCasdoorEnforcer(sqldb.DB)

	initDB()

	if parseFlags() {
		os.Exit(0)
	}

	r := gin.Default()
	// r.Use(middleware.CORS())
	r.Use(cors.New(cors.Options{
		AllowedOrigins:     []string{"*"},
		AllowCredentials:   true,
		Debug:              true,
		AllowedMethods:     []string{"GET", "POST", "OPTIONS", "DELETE"},
		AllowedHeaders:     []string{"*"},
		OptionsPassthrough: true,
	}))

	apiGroup := r.Group("/api/v1")
	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sessionController.SessionsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	authController.AuthRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sshKeyController.SshKeysRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	userController.UsersRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	initSwagger(r)

	r.Run(":8080")
}

func initSwagger(r *gin.Engine) {
	docs.SwaggerInfo.Title = "OCF API"
	docs.SwaggerInfo.Description = "This is an API to build and generate courses"
	docs.SwaggerInfo.Version = "0.0.1"
	docs.SwaggerInfo.Host = "localhost:8080"
	docs.SwaggerInfo.BasePath = "/api/v1"
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

func initDB() {

	if sqldb.DBType == "sqlite" {
		sqldb.DB = sqldb.DB.Debug()

		setupExternalUsersData()

		c := courseService.NewCourseService(sqldb.DB)
		courseOutputArray, _ := c.GetCourses()
		for _, courseOutput := range courseOutputArray {
			c.DeleteCourse(uuid.MustParse(courseOutput.CourseID_str))
		}
	}
}

func setupExternalUsersData() {
	testtools.DeleteAllObjects()
	testtools.SetupUsers()
	testtools.SetupGroups()
	testtools.SetupRoles()

	permissionsByRole, _ := casdoorsdk.GetPermissionsByRole("student")
	for _, permission := range permissionsByRole {
		fmt.Println(permission.Name)
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

	jsonConfigurationFilePath := "./src/configuration/conf.json"
	configuration := config.ReadJsonConfigurationFile(jsonConfigurationFilePath)

	course := getCourseFromProgramInputs(courseName, courseGitRepository, courseBranchGitRepository)

	setCourseThemeFromProgramInputs(&course, courseThemeName, courseThemeGitRepository, courseThemeBranchGitRepository)

	courseModels.FillCourseModelFromFiles(*courseName, &course)

	createdFile, err := course.WriteMd(&configuration)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Markdown file created: " + createdFile)

	errc := generator.SLIDE_ENGINE.CompileResources(&course)
	if errc != nil {
		log.Fatal(errc)
	}

	if !*config.DRY_RUN {
		generator.SLIDE_ENGINE.Run(&configuration, &course, courseType)
	}

	return true
}

func setCourseThemeFromProgramInputs(course *courseModels.Course, themeName *string, themeGitRepository *string, themeGitRepositoryBranch *string) {
	course.Theme = *themeName
	course.ThemeGitRepository = *themeGitRepository
	course.ThemeGitRepositoryBranch = *themeGitRepositoryBranch
}

func getCourseFromProgramInputs(courseName *string, courseGitRepository *string, courseGitRepositoryBranchName *string) courseModels.Course {
	isCourseGitRepository := (*courseGitRepository != "")

	// ToDo: Get loggued in User
	LogguedInUser, userErr := casdoorsdk.GetUserByEmail("1.supervisor@test.com")

	if userErr != nil {
		log.Fatal(userErr)
	}

	var errGetGitCourse error
	if isCourseGitRepository {
		c := courseService.NewCourseService(sqldb.DB)
		errGetGitCourse = c.GetGitCourse(*LogguedInUser, *courseName, *courseGitRepository, *courseGitRepositoryBranchName)
	}

	if errGetGitCourse != nil {
		log.Fatal(errGetGitCourse)
	}

	jsonCourseFilePath := config.COURSES_ROOT + *courseName + "/course.json"
	course := courseModels.ReadJsonCourseFile(jsonCourseFilePath)

	course.Owner = LogguedInUser
	course.OwnerID = LogguedInUser.Id
	course.FolderName = *courseName
	course.GitRepository = *courseGitRepository
	course.GitRepositoryBranch = *courseGitRepositoryBranchName
	return *course
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
