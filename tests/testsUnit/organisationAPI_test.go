package test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"soli/formations/src/auth/dto"
	organisationController "soli/formations/src/auth/routes/organisationRoutes"

	services "soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	mockServices "soli/formations/tests/servicesMocks"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockAddOrganisation(t *testing.T) {
	// 1. Setup
	gin.SetMode(gin.TestMode)
	router := gin.Default()

	//mockUserService := new(services.UserService) // Consider using a mock service here
	mockConfig := new(config.Configuration) // Mock configuration object

	mockOrganisationService := new(mockServices.MockOrganisationService)
	mockOrganisationService.On("CreateOrganisation", mock.Anything, mock.Anything).Return(&dto.OrganisationOutput{ID: uuid.New(), Name: "my_org_name", Groups: nil}, nil)

	AddOrganisation(mockOrganisationService, mockConfig, router, t)
}

func AddOrganisation(organisationService services.OrganisationService, mockConfig *config.Configuration, router *gin.Engine, t *testing.T) {
	controller := organisationController.NewOrganisationController(nil)

	router.POST("/organisations", controller.AddOrganisation)

	validRequestBody := `{"name": "mon_orga_de_test"}`

	request, _ := http.NewRequest(http.MethodPost, "/organisations", bytes.NewBufferString(validRequestBody))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)

}
