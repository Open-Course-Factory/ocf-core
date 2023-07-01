package test

import (
	"soli/formations/src/auth/dto"

	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	tests "soli/formations/tests"
)

func TestMockAddUser(t *testing.T) {
	// 1. Setup
	gin.SetMode(gin.TestMode)
	router := gin.Default()

	//mockUserService := new(services.UserService) // Consider using a mock service here
	mockConfig := new(config.Configuration) // Mock configuration object

	mockUserService := new(services.MockUserService)
	mockUserService.On("CreateUser", mock.Anything, mock.Anything).Return(&dto.UserOutput{ID: uuid.New()}, nil)

	// pass nil as we are using a mock user service
	// 2. Test case: Valid request body
	// We expect a StatusCreated (201) status
	// 3. Test case: Invalid request body
	tests.AddUser(mockUserService, mockConfig, router, t) // We expect a BadRequest (400) status
}
