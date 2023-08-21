package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"soli/formations/docs"

	config "soli/formations/src/configuration"
	marp "soli/formations/src/marp_integration"
	"soli/formations/src/middleware"

	authModels "soli/formations/src/auth/models"
	groupController "soli/formations/src/auth/routes/groupRoutes"
	loginController "soli/formations/src/auth/routes/loginRoutes"
	organisationController "soli/formations/src/auth/routes/organisationRoutes"
	roleController "soli/formations/src/auth/routes/roleRoutes"
	userController "soli/formations/src/auth/routes/userRoutes"
	"soli/formations/src/auth/services"
	courseModels "soli/formations/src/courses/models"
	courseController "soli/formations/src/courses/routes/courseRoutes"

	entityManagementServices "soli/formations/src/entityManagement/services"

	sqldb "soli/formations/src/db"
)

// @title User API
// @version 1.0
// @description This is a server to generate slides.
// @termsOfService TODO

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @contact.name Solution Libre
// @contact.url https://www.solution-libre.fr
// @contact.email contact@solution-libre.fr

// @host localhost:8000
// @BasePath /
func main() {

	if parseFlags() {
		os.Exit(0)
	}

	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.User{}).Name(), reflect.TypeOf(authModels.User{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.Group{}).Name(), reflect.TypeOf(authModels.Group{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.Role{}).Name(), reflect.TypeOf(authModels.Role{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.Organisation{}).Name(), reflect.TypeOf(authModels.Organisation{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.SshKey{}).Name(), reflect.TypeOf(authModels.SshKey{}))

	sqldb.ConnectDB()

	sqldb.DB.AutoMigrate(&authModels.User{}, &authModels.Role{}, &authModels.UserRoles{})
	sqldb.DB.AutoMigrate(&authModels.SshKey{})
	sqldb.DB.AutoMigrate(&authModels.Group{})
	sqldb.DB.AutoMigrate(&authModels.Organisation{})

	//sqldb.DB.SetupJoinTable(&authModels.User{}, "Roles", &authModels.UserRole{})
	sqldb.DB.AutoMigrate()

	sqldb.DB.AutoMigrate(&courseModels.Page{})
	sqldb.DB.AutoMigrate(&courseModels.Section{})
	sqldb.DB.AutoMigrate(&courseModels.Chapter{})
	sqldb.DB.AutoMigrate(&courseModels.Course{})

	initDB()

	r := gin.Default()
	r.Use(middleware.CORS())

	apiGroup := r.Group("/api/v1")
	userController.UsersRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	roleController.RolesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	groupController.GroupsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	organisationController.OrganisationsRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	loginController.LoginRoutes(apiGroup, &config.Configuration{}, sqldb.DB)
	loginController.RefreshRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	courseController.CoursesRoutes(apiGroup, &config.Configuration{}, sqldb.DB)

	initSwagger(r)

	r.Run(":8000")
}

func initSwagger(r *gin.Engine) {
	docs.SwaggerInfo.Title = "User API"
	docs.SwaggerInfo.Description = "This is a sample server for managing users"
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.Host = "localhost:8000"
	docs.SwaggerInfo.BasePath = "/api/v1"
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

func initDB() {
	genericService := services.NewGenericService(sqldb.DB)
	roleService := services.NewRoleService(sqldb.DB)
	roleService.SetupRoles()

	if sqldb.DBType == "sqlite" {
		sqldb.DB = sqldb.DB.Debug()

		users, _ := genericService.GetEntities(authModels.User{})

		if len(users) == 0 {

			userService := services.NewUserService(sqldb.DB)
			userService.CreateUserComplete("test@test.com", "test", "Tom", "Baggins")
			userService.CreateUserComplete("test2@test.com", "test2", "Bilbo", "Baggins")

			userTestAdminDto, _ := userService.CreateUserComplete("admin@test.com", "admin", "Gan", "Dalf")

			roleService := services.NewRoleService(sqldb.DB)
			roleInstanceAdminUuid, _ := roleService.GetRoleByType(authModels.RoleTypeInstanceAdmin)

			roleService.CreateUserRoleObjectAssociation(userTestAdminDto.ID, roleInstanceAdminUuid, uuid.Nil, "")

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

	errc := course.CompileResources(&configuration)
	if errc != nil {
		log.Fatal(errc)
	}

	marp.Run(&configuration, &course, courseType)

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
