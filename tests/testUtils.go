package test

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	sqldb "soli/formations/src/db"
	"testing"

	authModels "soli/formations/src/auth/models"

	loginController "soli/formations/src/auth/routes/loginRoutes"
	userController "soli/formations/src/auth/routes/userRoutes"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

var Router *gin.Engine
var MockConfig *config.Configuration
var UserService services.UserService
var OrganisationService services.OrganisationService

func SetupTest(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	// 1. Setup
	gin.SetMode(gin.TestMode)
	Router = gin.Default()

	//mockUserService := new(services.UserService) // Consider using a mock service here
	MockConfig = new(config.Configuration) // Mock configuration object

	sqldb.ConnectDB()

	sqldb.DB.AutoMigrate(&authModels.User{})
	sqldb.DB.AutoMigrate(&authModels.SshKey{})
	sqldb.DB.AutoMigrate(&authModels.Role{})
	sqldb.DB.AutoMigrate(&authModels.Group{})
	sqldb.DB.AutoMigrate(&authModels.Organisation{})
	sqldb.DB.AutoMigrate(&authModels.Permission{})
	sqldb.DB.AutoMigrate(&authModels.UserRole{})

	UserService = services.NewUserService(sqldb.DB)
	OrganisationService = services.NewOrganisationService(sqldb.DB)

	// Return a function to teardown the test
	return func(tb testing.TB) {
		log.Println("teardown test")
	}
}

func AddUser(userService services.UserService, mockConfig *config.Configuration, router *gin.Engine, t *testing.T) {
	controller := userController.NewUserController(nil, userService, mockConfig)

	router.POST("/users", controller.AddUser)

	validRequestBody := `{"email": "test@test.com", "password": "12345678", "firstName": "Test", "lastName": "User"}`
	invalidRequestBody := `{"email": "not a valid email", "password": "123456", "firstName": "Test", "lastName": "User"}`

	request, _ := http.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(validRequestBody))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)

	request, _ = http.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(invalidRequestBody))
	response = httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

// func AddGroup(groupService services.GroupService, mockConfig *config.Configuration, router *gin.Engine, t *testing.T) {
// 	controller := groupController.NewGroupController(nil, mockConfig)

// 	router.POST("/groups", controller.AddGroup)

// 	validRequestBody := `{"groupName": "mon_groupe_de_test", "password": "12345678", "firstName": "Test", "lastName": "User"}`
// 	//invalidRequestBody := `{"email": "not a valid email", "password": "123456", "firstName": "Test", "lastName": "User"}`

// 	request, _ := http.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(validRequestBody))
// 	response := httptest.NewRecorder()

// 	router.ServeHTTP(response, request)

// 	assert.Equal(t, http.StatusCreated, response.Code)

// 	request, _ = http.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(invalidRequestBody))
// 	response = httptest.NewRecorder()

// 	router.ServeHTTP(response, request)

// 	assert.Equal(t, http.StatusBadRequest, response.Code)
// }

func LoginUser(userService services.UserService, mockConfig *config.Configuration, router *gin.Engine, t *testing.T) string {
	controller := loginController.NewLoginController(sqldb.DB, mockConfig)

	router.POST("/login", controller.Login)

	validRequestBody := `{"email": "test@test.com", "password": "12345678"}`
	invalidRequestBody := `{"email": "test@test.com", "password": "12345679"}`

	request, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(validRequestBody))
	responseOK := httptest.NewRecorder()

	router.ServeHTTP(responseOK, request)

	assert.Equal(t, http.StatusOK, responseOK.Code)

	result := dto.UserTokens{}
	errUnmarshall := json.Unmarshal(responseOK.Body.Bytes(), &result)

	if errUnmarshall != nil {
		t.Error(errUnmarshall)
	}

	request, _ = http.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(invalidRequestBody))
	responseNotFound := httptest.NewRecorder()

	router.ServeHTTP(responseNotFound, request)

	assert.Equal(t, http.StatusNotFound, responseNotFound.Code)

	return result.Token
}
