package test

import (
	"log"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	"testing"

	sqldb "soli/formations/src/db"

	"github.com/gin-gonic/gin"

	tests "soli/formations/tests"
)

var router *gin.Engine
var mockConfig *config.Configuration
var userService services.UserService

func setupTest(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	// 1. Setup
	gin.SetMode(gin.TestMode)
	router = gin.Default()

	//mockUserService := new(services.UserService) // Consider using a mock service here
	mockConfig = new(config.Configuration) // Mock configuration object

	sqldb.ConnectDB()

	sqldb.DB.AutoMigrate(&authModels.User{})
	sqldb.DB.AutoMigrate(&authModels.Role{})
	sqldb.DB.AutoMigrate(&authModels.Group{})
	sqldb.DB.AutoMigrate(&authModels.Organisation{})
	sqldb.DB.AutoMigrate(&authModels.Permission{})

	userService = services.NewUserService(sqldb.DB)

	// Return a function to teardown the test
	return func(tb testing.TB) {
		log.Println("teardown test")
	}
}

func TestAddUser(t *testing.T) {
	teardownTest := setupTest(t)
	defer teardownTest(t)

	// pass nil as we are using a mock user service
	// 2. Test case: Valid request body
	// We expect a StatusCreated (201) status
	// 3. Test case: Invalid request body
	tests.AddUser(userService, mockConfig, router, t) // We expect a BadRequest (400) status
}
