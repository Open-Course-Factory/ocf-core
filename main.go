package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"

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
	courseModels "soli/formations/src/courses/models"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	sessionController "soli/formations/src/courses/routes/sessionRoutes"

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
}

func parseFlags() bool {

	const COURSE_FLAG = "c"
	const GIT_REPO_FLAG = "g"
	const THEME_FLAG = "t"
	const TYPE_FLAG = "e"
	const DRY_RUN_FLAG = "dry-run"
	const SLIDE_ENGINE_FLAG = "slide-engine"

	courseName := flag.String(COURSE_FLAG, "git", "trigram of the course you need to generate")
	courseGitRepository := flag.String(GIT_REPO_FLAG, "", "ssh git repository")
	courseTheme := flag.String(THEME_FLAG, "sdv", "theme used to generate the .md file in the right location")
	courseType := flag.String(TYPE_FLAG, "html", "type generated : html (default) or pdf")
	config.DRY_RUN = flag.Bool(DRY_RUN_FLAG, false, "if set true, the cli stops before calling slide generator")
	slideEngine := flag.String(SLIDE_ENGINE_FLAG, "slidev", "slide generator used, marp or slidev (default)")
	flag.Parse()

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

	course := getCourseFromProgramInputs(courseName, courseGitRepository, courseTheme)

	courseModels.CreateCourse(*courseName, &course)

	createdFile, err := course.WriteMd(&configuration)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Markdown file created: " + createdFile)

	errc := generator.SLIDE_ENGINE.CompileResources(&course, &configuration)
	if errc != nil {
		log.Fatal(errc)
	}

	if !*config.DRY_RUN {
		generator.SLIDE_ENGINE.Run(&configuration, &course, courseType)
	}

	return true
}

func getCourseFromProgramInputs(courseName *string, courseGitRepository *string, courseTheme *string) courseModels.Course {
	isCourseGitRepository := (*courseGitRepository != "")

	var errGetGitCourse error
	if isCourseGitRepository {
		c := courseService.NewCourseService(sqldb.DB)
		errGetGitCourse = c.GetGitCourse(casdoorsdk.User{Email: "1.supervisor@test.com", Name: "test"}, *courseName, *courseGitRepository)
	}

	if errGetGitCourse != nil {
		log.Fatal(errGetGitCourse)
	}

	jsonCourseFilePath := config.COURSES_ROOT + *courseName + "/course.json"
	course := courseModels.ReadJsonCourseFile(jsonCourseFilePath)

	if len(*courseTheme) > 0 {
		course.Theme = *courseTheme
	}
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
