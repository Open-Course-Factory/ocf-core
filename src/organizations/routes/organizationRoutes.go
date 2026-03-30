package routes

import (
	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"
	"soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OrganizationRoutes sets up custom organization routes
func OrganizationRoutes(rg *gin.RouterGroup, conf *config.Configuration, db *gorm.DB) {
	// Initialize services
	orgService := services.NewOrganizationService(db)
	importService := services.NewImportService(db)

	// Initialize controller (now includes group service)
	orgController := controller.NewOrganizationController(orgService, importService, db)

	// Initialize authentication middleware
	middleware := auth.NewAuthMiddleware(db)

	// Setup nested routes for organizations
	organizations := rg.Group("/organizations")
	{
		// Get members of a specific organization
		organizations.GET("/:id/members", middleware.AuthManagement(), orgController.GetOrganizationMembers)

		// Get groups of a specific organization
		organizations.GET("/:id/groups", middleware.AuthManagement(), orgController.GetOrganizationGroups)

		// Bulk import users, groups, and memberships
		organizations.POST("/:id/import", middleware.AuthManagement(), orgController.ImportOrganizationData)

		// Convert personal organization to team organization
		organizations.POST("/:id/convert-to-team", middleware.AuthManagement(), orgController.ConvertToTeam)

		// Regenerate passwords for group members
		organizations.POST("/:id/groups/:groupId/regenerate-passwords", middleware.AuthManagement(), orgController.RegenerateGroupMemberPasswords)

		// Backend assignment management
		organizations.GET("/:id/backends", middleware.AuthManagement(), orgController.GetOrganizationBackends)
		organizations.PUT("/:id/backends", middleware.AuthManagement(), orgController.UpdateOrganizationBackends)
	}
}
