package main

import (
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
	marp "soli/formations/src/marp_integration"
	"soli/formations/src/middleware"

	authDto "soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	groupController "soli/formations/src/auth/routes/groupRoutes"
	loginController "soli/formations/src/auth/routes/loginRoutes"
	organisationController "soli/formations/src/auth/routes/organisationRoutes"
	roleController "soli/formations/src/auth/routes/roleRoutes"
	userController "soli/formations/src/auth/routes/userRoutes"
	"soli/formations/src/auth/services"
	"soli/formations/src/auth/types"
	courseModels "soli/formations/src/courses/models"
	courseController "soli/formations/src/courses/routes/courseRoutes"

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

	sqldb.ConnectDB()

	sqldb.DB.AutoMigrate(&authModels.User{})
	sqldb.DB.AutoMigrate(&authModels.SshKey{})
	sqldb.DB.AutoMigrate(&authModels.Role{})
	sqldb.DB.AutoMigrate(&authModels.Group{})
	sqldb.DB.AutoMigrate(&authModels.Organisation{})

	sqldb.DB.SetupJoinTable(&authModels.User{}, "Roles", &authModels.UserRole{})
	sqldb.DB.AutoMigrate(&authModels.UserRole{})

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
	if sqldb.DBType == "sqlite" {
		genericService := services.NewGenericService(sqldb.DB)
		roleService := services.NewRoleService(sqldb.DB)
		//groupService := services.NewGroupService(sqldb.DB)
		users, _ := genericService.GetEntities(authModels.User{})

		if len(users) == 0 {
			// Roles should be pre-existant
			roleInstanceAdminInput := authDto.CreateRoleInput{RoleName: authModels.RoleTypeInstanceAdmin, Permissions: []types.Permission{types.PermissionTypeAll}}
			roleInstanceAdminOutput, _ := roleService.CreateRole(roleInstanceAdminInput, &config.Configuration{})

			roleOrganisationAdminInput := authDto.CreateRoleInput{RoleName: authModels.RoleTypeOrganisationAdmin, Permissions: []types.Permission{types.PermissionTypeAll}}
			roleService.CreateRole(roleOrganisationAdminInput, &config.Configuration{})

			// permissionInput := authDto.CreatePermissionInput{
			// 	PermissionTypes: []authModels.PermissionType{
			// 		authModels.PermissionTypeAll,
			// 	},
			// }
			// permissionService.CreatePermission(permissionInput)

			//permissionAssociationService, organisationOutputDto, permissionOutput := createUserComplete("test@test.com", "test", "Tom", "Baggins")
			createUserComplete("test@test.com", "test", "Tom", "Baggins")

			userTestAdminDto := createUserComplete("admin@test.com", "admin", "Gan", "Dalf")

			roleService.CreateUserRoleObjectAssociation(userTestAdminDto.ID, roleInstanceAdminOutput.ID, uuid.Nil, "")

			// groupInput := authDto.CreateGroupInput{GroupName: "groupTest"}
			// groupService.CreateGroup(groupInput)

			// groupInput2 := authDto.CreateGroupInput{GroupName: "groupTest2"}
			// groupService.CreateGroup(groupInput2)

			// permissionAssociationObject := authDto.PermissionAssociationObjectInput{
			// 	SubObjectID: organisationOutputDto.ID,
			// 	SubType:     reflect.TypeOf(models.Organisation{}).Name(),
			// }
			// permissionAssociationInput := authDto.PermissionAssociationInput{
			// 	PermissionID:                 permissionOutput.ID.String(),
			// 	PermissionAssociationObjects: []authDto.PermissionAssociationObjectInput{permissionAssociationObject},
			// }

			// permissionAssociationService.CreatePermissionAssociation(permissionAssociationInput)

		}
	}
}

func createUserComplete(email string, password string, firstName string, lastName string) *authDto.UserOutput {

	userService := services.NewUserService(sqldb.DB)

	userInput := authDto.CreateUserInput{Email: email, Password: password, FirstName: firstName, LastName: lastName}
	userOutputDto, _ := userService.CreateUser(userInput, &config.Configuration{})

	organisationService := services.NewOrganisationService(sqldb.DB)

	organisationInput := authDto.CreateOrganisationInput{Name: lastName + "_org"}
	organisationOutputDto, _ := organisationService.CreateOrganisation(organisationInput, &config.Configuration{})

	roleService := services.NewRoleService(sqldb.DB)
	roleId, _ := roleService.GetRoleByType(authModels.RoleTypeOrganisationAdmin)

	roleService.CreateUserRoleObjectAssociation(userOutputDto.ID, roleId, organisationOutputDto.ID, "Organisation")

	return userOutputDto
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
