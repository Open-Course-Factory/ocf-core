package test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/auth/middleware"
	organisationController "soli/formations/src/auth/routes/organisationRoutes"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	sqldb "soli/formations/src/db"
	tests "soli/formations/tests"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAddOrganisation(t *testing.T) {
	teardownTest := tests.SetupTest(t)
	defer teardownTest(t)

	token := tests.LoginUser(tests.UserService, tests.MockConfig, tests.Router, t)
	AddOrganisation(tests.OrganisationService, tests.MockConfig, token, tests.Router, t)
}

func AddOrganisation(organisationService services.OrganisationService, mockConfig *config.Configuration, token string, router *gin.Engine, t *testing.T) {
	controller := organisationController.NewOrganisationController(sqldb.DB, mockConfig)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: mockConfig,
	}

	// permissionMiddleware := &middleware.PermissionsMiddleware{
	// 	DB: sqldb.DB,
	// }

	router.POST("/organisations", authMiddleware.CheckIsLogged(), controller.AddOrganisation)

	validRequestBody := `{"name": "mon_orga_de_test"}`
	//invalidRequestBody := `{"email": "not a valid email", "password": "123456", "firstName": "Test", "lastName": "User"}`

	request, _ := http.NewRequest(http.MethodPost, "/organisations", bytes.NewBufferString(validRequestBody))
	request.Header.Set("Authorization", "bearer "+token)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)

}
