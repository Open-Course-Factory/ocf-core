package test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	"testing"

	userController "soli/formations/src/auth/routes/userRoutes"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

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
