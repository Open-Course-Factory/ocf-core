package auth_tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	sqldb "soli/formations/src/db"
	usernameController "soli/formations/src/labs/routes/usernameRoutes"
	test_tools "soli/formations/tests/testTools"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestUsernameCreate(t *testing.T) {
	teardownTest := test_tools.SetupFunctionnalTests(t)
	defer teardownTest(t)

	usernameController := usernameController.NewUsernameController(sqldb.DB)
	router := gin.Default()
	router.POST("/api/v1/usernames/", usernameController.AddUsername)

	body := []byte(`{
		"username": "tom"
	}`)
	req, err := http.NewRequest("POST", "/api/v1/usernames/", bytes.NewBuffer(body))
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	body = []byte(`{
		"username": "tom"
	}`)
	req2, err2 := http.NewRequest("POST", "/api/v1/usernames/", bytes.NewBuffer(body))
	assert.NoError(t, err2)

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}
