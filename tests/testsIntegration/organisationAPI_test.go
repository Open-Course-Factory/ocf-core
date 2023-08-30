package test

import (
	"encoding/json"
	"net/http"
	"testing"

	"soli/formations/src/auth/middleware"
	organisationController "soli/formations/src/auth/routes/organisationRoutes"
	"soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	tests "soli/formations/tests"

	"github.com/stretchr/testify/assert"
)

func TestAddOrganisation(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	AddOrganisation(t)
}

func TestGetOrganisations(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	GetOrganisations(t)
}

func TestAdminGetOrganisations(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	GetOrganisationsAdmin(t)
}

func TestFailGetOrganisations(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	FailGetOrganisations(t)
}

func AddOrganisation(t *testing.T) {
	router := tests.NewGin()
	controller := organisationController.NewOrganisationController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	genericService := services.NewGenericService(sqldb.DB)
	permissionMiddleware := middleware.NewPermissionsMiddleware(sqldb.DB, genericService)
	token := tests.LoginUser("test@test.com", "test", tests.MockConfig, t)

	router.POST("/api/v1/organisations", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), controller.AddOrganisation)
	responseAddOrgs := tests.PerformRequest(router, http.MethodPost, "/api/v1/organisations", tests.WithHeader("Authorization", "bearer "+token), tests.WithBody(`{"name": "mon_orga_de_test"}`))

	assert.Equal(t, http.StatusCreated, responseAddOrgs.Code)

}

func GetOrganisations(t *testing.T) {
	router := tests.NewGin()
	controller := organisationController.NewOrganisationController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	token := tests.LoginUser("test@test.com", "test", tests.MockConfig, t)

	router.GET("api/v1/organisations", authMiddleware.CheckIsLogged(), controller.GetOrganisations)
	responseGetAllOrgs := tests.PerformRequest(router, http.MethodGet, "/api/v1/organisations", tests.WithHeader("Authorization", "bearer "+token))

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

	responseGetSingleOrg := tests.PerformRequest(router, http.MethodGet, "/api/v1/organisations/"+orgId, tests.WithHeader("Authorization", "bearer "+token))

	assert.Equal(t, http.StatusOK, responseGetSingleOrg.Code)

}

func GetOrganisationsAdmin(t *testing.T) {
	router := tests.NewGin()
	controller := organisationController.NewOrganisationController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	token := tests.LoginUser("admin@test.com", "admin", tests.MockConfig, t)

	router.GET("api/v1/organisations", authMiddleware.CheckIsLogged(), controller.GetOrganisations)
	responseGetAllOrgs := tests.PerformRequest(router, http.MethodGet, "/api/v1/organisations", tests.WithHeader("Authorization", "bearer "+token))

	assert.Equal(t, http.StatusOK, responseGetAllOrgs.Code)

	//here we need to get the response body to do some tests on it
	res := responseGetAllOrgs.Body.Bytes()
	// here we need to extract result in a map
	var resultMap []map[string]interface{}
	json.Unmarshal(res, &resultMap)

	assert.Equal(t, 3, len(resultMap))

}

func FailGetOrganisations(t *testing.T) {
	router := tests.NewGin()
	controller := organisationController.NewOrganisationController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	tokenAdmin := tests.LoginUser("admin@test.com", "admin", tests.MockConfig, t)

	router.GET("api/v1/organisations", authMiddleware.CheckIsLogged(), controller.GetOrganisations)
	responseGetAllOrgs := tests.PerformRequest(router, http.MethodGet, "/api/v1/organisations", tests.WithHeader("Authorization", "bearer "+tokenAdmin))

	assert.Equal(t, http.StatusOK, responseGetAllOrgs.Code)

	//here we need to get the response body to do some tests on it
	res := responseGetAllOrgs.Body.Bytes()
	// here we need to extract result in a map
	var resultMap []map[string]interface{}
	json.Unmarshal(res, &resultMap)

	assert.Equal(t, 3, len(resultMap))

	orgId := resultMap[2]["id"].(string)

	genericService := services.NewGenericService(sqldb.DB)
	permissionMiddleware := middleware.NewPermissionsMiddleware(sqldb.DB, genericService)

	router = tests.NewGin()

	tokenSimpleUser := tests.LoginUser("test@test.com", "test", tests.MockConfig, t)

	router.GET("/api/v1/organisations/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), controller.GetOrganisation)

	responseGetForbiddenOrg := tests.PerformRequest(router, http.MethodGet, "/api/v1/organisations/"+orgId, tests.WithHeader("Authorization", "bearer "+tokenSimpleUser))

	assert.Equal(t, http.StatusForbidden, responseGetForbiddenOrg.Code)

}
