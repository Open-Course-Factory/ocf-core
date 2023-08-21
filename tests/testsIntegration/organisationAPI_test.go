package test

import (
	"bytes"
	"encoding/json"
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
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	token := tests.LoginUser("test@test.com", "test", tests.MockConfig, tests.Router, t)
	AddOrganisation(tests.OrganisationService, tests.MockConfig, token, tests.Router, t)
}

func TestGetOrganisations(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	token := tests.LoginUser("test@test.com", "test", tests.MockConfig, tests.Router, t)
	GetOrganisations(tests.OrganisationService, tests.MockConfig, token, t)
}

func AddOrganisation(organisationService services.OrganisationService, mockConfig *config.Configuration, token string, router *gin.Engine, t *testing.T) {
	controller := organisationController.NewOrganisationController(sqldb.DB, organisationService)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: mockConfig,
	}

	genericService := services.NewGenericService(sqldb.DB)
	permissionMiddleware := middleware.NewPermissionsMiddleware(sqldb.DB, genericService)

	router.POST("/organisations", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), controller.AddOrganisation)

	validRequestBody := `{"name": "mon_orga_de_test"}`
	//invalidRequestBody := `{"email": "not a valid email", "password": "123456", "firstName": "Test", "lastName": "User"}`

	request, _ := http.NewRequest(http.MethodPost, "/organisations", bytes.NewBufferString(validRequestBody))
	request.Header.Set("Authorization", "bearer "+token)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)

}

func GetOrganisations(organisationService services.OrganisationService, mockConfig *config.Configuration, token string, t *testing.T) {
	router := tests.NewGin()
	controller := organisationController.NewOrganisationController(sqldb.DB, organisationService)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: mockConfig,
	}

	router.GET("api/v1/organisations", authMiddleware.CheckIsLogged(), controller.GetOrganisations)

	responseGetAllOrgs := tests.PerformRequest(router, http.MethodGet, "/api/v1/organisations", token)

	assert.Equal(t, http.StatusOK, responseGetAllOrgs.Code)

	//here we need to get the response body to do some tests on it
	res := responseGetAllOrgs.Body.Bytes()
	// here we need to extract result in a map
	var resultMap []map[string]interface{}
	json.Unmarshal(res, &resultMap)

	assert.Equal(t, 1, len(resultMap))

	orgId := resultMap[0]["id"].(string)

	genericService := services.NewGenericService(sqldb.DB)
	permissionMiddleware := middleware.NewPermissionsMiddleware(sqldb.DB, genericService)

	router = tests.NewGin()

	router.GET("/api/v1/organisations/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), controller.GetOrganisation)

	responseGetSingleOrg := tests.PerformRequest(router, http.MethodGet, "/api/v1/organisations/"+orgId, token)

	assert.Equal(t, http.StatusOK, responseGetSingleOrg.Code)

}
