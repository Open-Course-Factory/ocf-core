package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"soli/formations/docs"

	config "soli/formations/src/configuration"
	generator "soli/formations/src/generationEngine"
	marp "soli/formations/src/generationEngine/marp_integration"
	slidev "soli/formations/src/generationEngine/slidev_integration"
	"soli/formations/src/middleware"
	testtools "soli/formations/src/testTools"

	authController "soli/formations/src/auth"
	authDto "soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	sshKeyController "soli/formations/src/auth/routes/sessionRoutes"
	courseModels "soli/formations/src/courses/models"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	sessionController "soli/formations/src/courses/routes/sessionRoutes"

	authService "soli/formations/src/auth/services"
	courseService "soli/formations/src/courses/services"

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

	authController.InitCasdoorConnection()

	sqldb.InitDBConnection()

	sqldb.DB.AutoMigrate()

	sqldb.DB.AutoMigrate(&courseModels.Page{})
	sqldb.DB.AutoMigrate(&courseModels.Section{})
	sqldb.DB.AutoMigrate(&courseModels.Chapter{})
	sqldb.DB.AutoMigrate(&courseModels.Course{})

	sqldb.DB.AutoMigrate(&authModels.SshKey{})

	initDB()

	if parseFlags() {
		os.Exit(0)
	}

	r := gin.Default()
	r.Use(middleware.CORS())

	apiGroup := r.Group("/api/v1")
	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sessionController.SessionsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	authController.AuthRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	sshKeyController.SshKeysRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

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

		user, _ := casdoorsdk.GetUserByEmail("1.supervisor@test.com")
		sks := authService.NewSshKeyService(sqldb.DB)

		keysOutputArray, _ := sks.GetAllKeys()
		for _, keysOutput := range *keysOutputArray {
			sks.DeleteKey(keysOutput.Id.String())
		}

		sshPrivateKey := `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----`

		key := authDto.CreateSshKeyInput{
			KeyName:    "test Thomas",
			UserId:     uuid.MustParse(user.Id),
			PrivateKey: sshPrivateKey,
		}

		sks.AddUserSshKey(key)

	}
}

func setupExternalUsersData() {
	testtools.DeleteAllObjects()
	testtools.SetupUsers()
	testtools.SetupGroups()
	testtools.SetupRoles()
	testtools.SetupPermissions()

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
	return course
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
