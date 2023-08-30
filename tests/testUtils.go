package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	sqldb "soli/formations/src/db"
	"strings"
	"testing"

	authModels "soli/formations/src/auth/models"

	loginController "soli/formations/src/auth/routes/loginRoutes"
	userController "soli/formations/src/auth/routes/userRoutes"

	entityManagementServices "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var Router *gin.Engine
var MockConfig *config.Configuration
var UserService services.UserService
var RoleService services.RoleService
var OrganisationService services.OrganisationService
var GroupService services.GroupService

func SetupFunctionnalTests(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.User{}).Name(), reflect.TypeOf(authModels.User{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.Group{}).Name(), reflect.TypeOf(authModels.Group{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.Role{}).Name(), reflect.TypeOf(authModels.Role{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.Organisation{}).Name(), reflect.TypeOf(authModels.Organisation{}))
	entityManagementServices.GlobalEntityRegistrationService.RegisterEntityType(reflect.TypeOf(authModels.SshKey{}).Name(), reflect.TypeOf(authModels.SshKey{}))

	// 1. Setup
	Router = NewGin()

	//mockUserService := new(services.UserService) // Consider using a mock service here
	MockConfig = new(config.Configuration) // Mock configuration object

	sqldb.ConnectDB()

	sqldb.DB.AutoMigrate(&authModels.User{}, &authModels.Role{}, &authModels.UserRoles{})
	sqldb.DB.AutoMigrate(&authModels.SshKey{})
	sqldb.DB.AutoMigrate(&authModels.Group{})
	sqldb.DB.AutoMigrate(&authModels.Organisation{})

	RoleService = services.NewRoleService(sqldb.DB)
	RoleService.SetupRoles()

	UserService = services.NewUserService(sqldb.DB)
	OrganisationService = services.NewOrganisationService(sqldb.DB)
	GroupService = services.NewGroupService(sqldb.DB)

	// 3 users
	// 2 lambda users with 1 organisation and 1 group each
	// 1 admin user
	userService := services.NewUserService(sqldb.DB)
	userService.CreateUserComplete("test@test.com", "test", "Tom", "Baggins")
	userService.CreateUserComplete("test2@test.com", "test2", "Bilbo", "Baggins")

	userTestAdminDto, _ := userService.CreateUserComplete("admin@test.com", "admin", "Gan", "Dalf")

	roleService := services.NewRoleService(sqldb.DB)
	roleInstanceAdminUuid, _ := roleService.GetRoleByType(authModels.RoleTypeInstanceAdmin)

	roleService.CreateUserRoleObjectAssociation(userTestAdminDto.ID, roleInstanceAdminUuid, uuid.Nil, "")

	// Return a function to teardown the test
	return func(tb testing.TB) {
		log.Println("teardown test")
		removeDBFile()
	}
}

func removeDBFile() {
	filePath := sqldb.DB_FILE

	// Use os.Remove to delete the file
	err := os.Remove(filePath)

	// Check if there was an error
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("File deleted successfully!")
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

func LoginUser(email string, password string, mockConfig *config.Configuration, t *testing.T) string {
	router := NewGin()
	controller := loginController.NewLoginController(sqldb.DB, mockConfig)

	router.POST("/api/v1/login", controller.Login)

	validRequestBody := `{"email": "` + email + `", "password": "` + password + `"}`
	responseValidLoginUser := PerformRequest(router, http.MethodPost, "/api/v1/login", WithBody(validRequestBody))
	assert.Equal(t, http.StatusOK, responseValidLoginUser.Code)

	result := dto.UserTokens{}
	errUnmarshall := json.Unmarshal(responseValidLoginUser.Body.Bytes(), &result)

	if errUnmarshall != nil {
		t.Error(errUnmarshall)
	}

	invalidRequestBody := `{"email": "test@test.com", "password": "` + password + `9"}`
	responseInvalidLoginUser := PerformRequest(router, http.MethodPost, "/api/v1/login", WithBody(invalidRequestBody))

	assert.Equal(t, http.StatusNotFound, responseInvalidLoginUser.Code)

	return result.Token
}

// NewGin returns a new gin engine with test mode enabled.
func NewGin() *gin.Engine {
	engine := gin.Default()
	gin.SetMode(gin.TestMode)
	return engine
}

type Option = func(*http.Request)

func WithBody(body string) Option {
	return func(request *http.Request) {
		request.Body = io.NopCloser(strings.NewReader(body))
	}
}

func WithHeader(name, value string) Option {
	return func(request *http.Request) {
		request.Header.Add(name, value)
	}
}

func PerformRequest(handler http.Handler, method, path string, options ...Option) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	return PerformRequestWithRecorder(recorder, handler, method, path, options...)
}

func PerformRequestWithRecorder(recorder *httptest.ResponseRecorder, r http.Handler, method, path string, options ...Option) *httptest.ResponseRecorder {
	request, err := http.NewRequest(method, path, nil)
	if err != nil {
		panic(err)
	}
	for _, opt := range options {
		opt(request)
	}
	r.ServeHTTP(recorder, request)
	return recorder
}
