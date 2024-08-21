package auth_tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	authController "soli/formations/src/auth"
	authDto "soli/formations/src/auth/dto"
	authRegistration "soli/formations/src/auth/entityRegistration"
	usernameController "soli/formations/src/auth/routes/usernameRoutes"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	labRegistration "soli/formations/src/labs/entityRegistration"
	test_tools "soli/formations/tests/testTools"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestUsernameAuth(t *testing.T) {
	ems.GlobalEntityRegistrationService.RegisterEntity(authRegistration.UsernameRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(labRegistration.MachineRegistration{})
	teardownTest := test_tools.SetupFunctionnalTests(t)
	defer teardownTest(t)

	loginAdminOutput := testLogin("1.supervisor@test.com", "test", t)
	loginUserOutput := testLogin("1.student@test.com", "test", t)

	recAdmin := createUsernameLoggedIn(loginAdminOutput, t)
	assert.Equal(t, http.StatusCreated, recAdmin.Code)

	recUser := createUsernameLoggedIn(loginUserOutput, t)
	assert.Equal(t, http.StatusCreated, recUser.Code)

	// body = []byte(`{
	// 	"username": "tom"
	// }`)
	// req2, err2 := http.NewRequest("POST", "/api/v1/usernames/", bytes.NewBuffer(body))
	// assert.NoError(t, err2)

	// rec2 := httptest.NewRecorder()
	// router.ServeHTTP(rec2, req2)

	// assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

func createUsernameLoggedIn(loginOutput authDto.LoginOutput, t *testing.T) *httptest.ResponseRecorder {
	router := gin.Default()

	middleware := authController.NewAuthMiddleware(sqldb.DB)
	usernameController := usernameController.NewUsernameController(sqldb.DB)
	router.POST("/api/v1/usernames/", middleware.AuthManagement(), usernameController.AddUsername)

	body := []byte(`{
		"username": "tom"
	}`)

	req, err := http.NewRequest("POST", "/api/v1/usernames/", bytes.NewBuffer(body))
	req.Header.Set("Authorization", loginOutput.AccessToken)
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func testLogin(email, password string, t *testing.T) authDto.LoginOutput {
	authController := authController.NewAuthController()
	router := gin.Default()

	router.POST("/api/v1/login/", authController.Login)
	body := []byte(`{
		"email": "` + email + `",
		"password": "` + password + `"
	}`)

	req, err := http.NewRequest("POST", "/api/v1/login/", bytes.NewBuffer(body))
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	loginOutput := authDto.LoginOutput{}
	bodyResp, _ := io.ReadAll(rec.Body)
	json.Unmarshal(bodyResp, &loginOutput)
	return loginOutput
}
