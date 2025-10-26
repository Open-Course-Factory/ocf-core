package routes

import (
	config "soli/formations/src/configuration"
	"soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OrganizationRoutes sets up custom organization routes
func OrganizationRoutes(rg *gin.RouterGroup, conf *config.Configuration, db *gorm.DB) {
	// Initialize organization service and controller
	orgService := services.NewOrganizationService(db)
	orgController := controller.NewOrganizationController(orgService)

	// Setup nested routes for organizations
	organizations := rg.Group("/organizations")
	{
		// Get members of a specific organization
		organizations.GET("/:id/members", orgController.GetOrganizationMembers)
	}
}
