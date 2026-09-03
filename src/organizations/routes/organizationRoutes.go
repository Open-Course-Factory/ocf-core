package routes

import (
	auth "soli/formations/src/auth"
	authServices "soli/formations/src/auth/services"
	config "soli/formations/src/configuration"
	"soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/services"
	paymentServices "soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OrganizationRoutes sets up custom organization routes
func OrganizationRoutes(rg *gin.RouterGroup, conf *config.Configuration, db *gorm.DB) {
	// Initialize services
	orgService := services.NewOrganizationService(db)
	casdoorClient := authServices.NewCasdoorUserClient()
	userService := authServices.NewUserService(casdoorClient, paymentServices.NewPaymentDeletionHelper(db))
	deletionService := authServices.NewUserDeletionService(db, userService)
	offboardingService := services.NewMemberOffboardingService(db, casdoorClient, paymentServices.NewBulkLicenseService(db), deletionService)
	importService := services.NewImportService(db, casdoorClient, offboardingService)

	orgController := controller.NewOrganizationController(orgService, importService, offboardingService, deletionService, db)

	// Initialize authentication middleware
	middleware := auth.NewAuthMiddleware(db)

	// Setup nested routes for organizations
	organizations := rg.Group("/organizations")
	{
		// Get members of a specific organization
		organizations.GET("/:id/members", middleware.AuthManagement(), orgController.GetOrganizationMembers)

		// Member offboarding lifecycle
		organizations.POST("/:id/members/offboard", middleware.AuthManagement(), orgController.OffboardMembers)
		organizations.POST("/:id/members/:userId/reinstate", middleware.AuthManagement(), orgController.ReinstateMember)
		organizations.POST("/:id/members/:userId/erase", middleware.AuthManagement(), orgController.EraseMember)

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
