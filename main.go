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
	"soli/formations/src/middleware"
	slidev "soli/formations/src/slidev_integration"
	testtools "soli/formations/src/testTools"

	authController "soli/formations/src/auth"
	courseModels "soli/formations/src/courses/models"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	sessionController "soli/formations/src/courses/routes/sessionRoutes"

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

	if parseFlags() {
		os.Exit(0)
	}

	authController.InitCasdoorConnection()

	sqldb.InitDBConnection()

	sqldb.DB.AutoMigrate()

	sqldb.DB.AutoMigrate(&courseModels.Page{})
	sqldb.DB.AutoMigrate(&courseModels.Section{})
	sqldb.DB.AutoMigrate(&courseModels.Chapter{})
	sqldb.DB.AutoMigrate(&courseModels.Course{})

	initDB()

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
		// setup casdoor test entities

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
	// c := courseService.NewCourseService(sqldb.DB)
	// c.GetGitCourse("git@usine.solution-libre.fr:formations/formations_marp.git")

	const COURSE_FLAG = "c"
	const THEME_FLAG = "t"
	const TYPE_FLAG = "e"

	courseName := flag.String(COURSE_FLAG, "git", "trigram of the course you need to generate")
	courseTheme := flag.String(THEME_FLAG, "sdv", "theme used to generate the .md file in the right location")
	courseType := flag.String(TYPE_FLAG, "html", "type generated : html (default) or pdf")
	flag.Parse()

	if !isFlagPassed(COURSE_FLAG) || !isFlagPassed(THEME_FLAG) || !isFlagPassed(TYPE_FLAG) {
		return false
	}

	jsonConfigurationFilePath := "./conf/conf.json"
	configuration := config.ReadJsonConfigurationFile(jsonConfigurationFilePath)

	jsonCourseFilePath := config.COURSES_ROOT + *courseName + ".json"
	course := courseModels.ReadJsonCourseFile(jsonCourseFilePath)

	if len(*courseTheme) > 0 {
		course.Theme = *courseTheme
	}

	courseModels.CreateCourse(&course)

	createdFile, err := course.WriteMd(&configuration)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Markdown file created: " + createdFile)

	errc := slidev.CompileResources(&course, &configuration)
	if errc != nil {
		log.Fatal(errc)
	}

	//marp.Run(&configuration, &course, courseType)

	slidev.Run(&configuration, &course, courseType)

	fmt.Println(courseType)

	return true
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
