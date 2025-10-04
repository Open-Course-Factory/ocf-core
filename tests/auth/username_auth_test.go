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
	sqldb "soli/formations/src/db"
	usernameController "soli/formations/src/labs/routes/usernameRoutes"

	test_tools "soli/formations/tests/testTools"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestUsernameAuth(t *testing.T) {
	teardownTest := test_tools.SetupFunctionnalTests(t)
	defer teardownTest(t)

	loginAdminOutput := testLogin("1.supervisor@test.com", "test", t)
	loginUserOutput := testLogin("1.student@test.com", "test", t)

	recAdmin := createUsernameLoggedIn(loginAdminOutput, t)
	assert.Equal(t, http.StatusCreated, recAdmin.Code)

	recUser := createUsernameLoggedIn(loginUserOutput, t)
	assert.Equal(t, http.StatusCreated, recUser.Code)
}

func createUsernameLoggedIn(loginOutput authDto.LoginOutput, t *testing.T) *httptest.ResponseRecorder {
	router := gin.Default()

	middleware := authController.NewAuthMiddleware(sqldb.DB)
	usernameController := usernameController.NewUsernameController(sqldb.DB)
	router.POST("/api/v1/usernames/", middleware.AuthManagement(), usernameController.AddUsername)

	body := []byte(`{
		"username": "` + loginOutput.UserName + `"
	}`)

	req, err := http.NewRequest("POST", "/api/v1/usernames/", bytes.NewBuffer(body))
	req.Header.Set("Authorization", loginOutput.AccessToken)
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Debug: print response body if not 201
	if rec.Code != http.StatusCreated {
		t.Logf("Auth failed with status %d. Response body: %s", rec.Code, rec.Body.String())
		t.Logf("Access token used: %s", loginOutput.AccessToken[:50]+"...")
	}

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
