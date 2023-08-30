package test

import (
	"encoding/json"
	"net/http"
	"testing"

	"soli/formations/src/auth/middleware"
	groupController "soli/formations/src/auth/routes/groupRoutes"
	"soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	tests "soli/formations/tests"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAddGroup(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	AddGroup(t)
}

func TestGetGroups(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	GetGroups(t)
}

func TestAdminGetGroups(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	GetGroupsAdmin(t)
}

func TestFailGetGroups(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	FailGetGroups(t)
}

func AddGroup(t *testing.T) {
	router := tests.NewGin()
	controller := groupController.NewGroupController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	token := tests.LoginUser("test@test.com", "test", tests.MockConfig, t)

	orgId := getOrgIdWithExistingGroup(router, authMiddleware, controller, token, t)

	router = tests.NewGin()

	genericService := services.NewGenericService(sqldb.DB)
	permissionMiddleware := middleware.NewPermissionsMiddleware(sqldb.DB, genericService)

	payload := `{"groupName": "mon_groupe_de_test", "organisation": "` + orgId + `"}`
	router.POST("/api/v1/groups", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), controller.AddGroup)
	responseAddOrgs := tests.PerformRequest(router, http.MethodPost, "/api/v1/groups", tests.WithHeader("Authorization", "bearer "+token), tests.WithBody(payload))

	assert.Equal(t, http.StatusCreated, responseAddOrgs.Code)

}

func getOrgIdWithExistingGroup(router *gin.Engine, authMiddleware *middleware.AuthMiddleware, controller groupController.GroupController, token string, t *testing.T) string {
	router.GET("api/v1/groups", authMiddleware.CheckIsLogged(), controller.GetGroups)
	responseGetAllGroups := tests.PerformRequest(router, http.MethodGet, "/api/v1/groups", tests.WithHeader("Authorization", "bearer "+token))

	assert.Equal(t, http.StatusOK, responseGetAllGroups.Code)

	res := responseGetAllGroups.Body.Bytes()

	var resultMap []map[string]interface{}
	json.Unmarshal(res, &resultMap)

	assert.Equal(t, 1, len(resultMap))

	orgId := resultMap[0]["organisation"].(string)
	return orgId
}

func GetGroups(t *testing.T) {
	router := tests.NewGin()
	controller := groupController.NewGroupController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	token := tests.LoginUser("test@test.com", "test", tests.MockConfig, t)

	router.GET("api/v1/groups", authMiddleware.CheckIsLogged(), controller.GetGroups)
	responseGetAllGroups := tests.PerformRequest(router, http.MethodGet, "/api/v1/groups", tests.WithHeader("Authorization", "bearer "+token))

	assert.Equal(t, http.StatusOK, responseGetAllGroups.Code)

	//here we need to get the response body to do some tests on it
	res := responseGetAllGroups.Body.Bytes()
	// here we need to extract result in a map
	var resultMap []map[string]interface{}
	json.Unmarshal(res, &resultMap)

	assert.Equal(t, 1, len(resultMap))

	grpId := resultMap[0]["id"].(string)

	genericService := services.NewGenericService(sqldb.DB)
	permissionMiddleware := middleware.NewPermissionsMiddleware(sqldb.DB, genericService)

	router = tests.NewGin()

	router.GET("/api/v1/groups/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), controller.GetGroup)

	responseGetSingleGroup := tests.PerformRequest(router, http.MethodGet, "/api/v1/groups/"+grpId, tests.WithHeader("Authorization", "bearer "+token))

	assert.Equal(t, http.StatusOK, responseGetSingleGroup.Code)

}

func GetGroupsAdmin(t *testing.T) {
	router := tests.NewGin()
	controller := groupController.NewGroupController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	token := tests.LoginUser("admin@test.com", "admin", tests.MockConfig, t)

	router.GET("api/v1/groups", authMiddleware.CheckIsLogged(), controller.GetGroups)
	responseGetAllOrgs := tests.PerformRequest(router, http.MethodGet, "/api/v1/groups", tests.WithHeader("Authorization", "bearer "+token))

	assert.Equal(t, http.StatusOK, responseGetAllOrgs.Code)

	//here we need to get the response body to do some tests on it
	res := responseGetAllOrgs.Body.Bytes()
	// here we need to extract result in a map
	var resultMap []map[string]interface{}
	json.Unmarshal(res, &resultMap)

	assert.Equal(t, 3, len(resultMap))

}

func FailGetGroups(t *testing.T) {
	router := tests.NewGin()
	controller := groupController.NewGroupController(sqldb.DB)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     sqldb.DB,
		Config: tests.MockConfig,
	}

	tokenAdmin := tests.LoginUser("admin@test.com", "admin", tests.MockConfig, t)

	router.GET("api/v1/groups", authMiddleware.CheckIsLogged(), controller.GetGroups)
	responseGetAllOrgs := tests.PerformRequest(router, http.MethodGet, "/api/v1/groups", tests.WithHeader("Authorization", "bearer "+tokenAdmin))

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

	router.GET("/api/v1/groups/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), controller.GetGroup)

	responseGetForbiddenOrg := tests.PerformRequest(router, http.MethodGet, "/api/v1/groups/"+orgId, tests.WithHeader("Authorization", "bearer "+tokenSimpleUser))

	assert.Equal(t, http.StatusForbidden, responseGetForbiddenOrg.Code)

}
