package routes

import (
	"log"

	auth "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"
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

	// Setup Casbin permissions for custom organization routes
	setupOrganizationCustomRoutePermissions()

	// Setup nested routes for organizations
	organizations := rg.Group("/organizations")
	{
		// Get members of a specific organization
		organizations.GET("/:id/members", middleware.AuthManagement(), orgController.GetOrganizationMembers)

		// Get groups of a specific organization
		organizations.GET("/:id/groups", middleware.AuthManagement(), orgController.GetOrganizationGroups)

		// Bulk import users, groups, and memberships
		organizations.POST("/:id/import", middleware.AuthManagement(), orgController.ImportOrganizationData)
	}
}

// setupOrganizationCustomRoutePermissions configures Casbin policies for custom organization endpoints
func setupOrganizationCustomRoutePermissions() {
	// Define roles that can access organization custom routes
	roles := []string{
		"administrator",
		"supervisor",
		"trainer",
		"member",
		"user",
		"admin", // Casdoor admin role
	}

	// Add permissions for /members endpoint (read-only)
	for _, role := range roles {
		_, err := casdoor.Enforcer.AddPolicy(role, "/api/v1/organizations/*/members", "GET")
		if err != nil {
			log.Printf("Failed to add policy for role %s on /members: %v", role, err)
		}
	}

	// Add permissions for /groups endpoint (read-only)
	for _, role := range roles {
		_, err := casdoor.Enforcer.AddPolicy(role, "/api/v1/organizations/*/groups", "GET")
		if err != nil {
			log.Printf("Failed to add policy for role %s on /groups: %v", role, err)
		}
	}

	// Add permissions for /import endpoint (admin/supervisor only)
	adminRoles := []string{"administrator", "supervisor", "admin", "trainer"}
	for _, role := range adminRoles {
		_, err := casdoor.Enforcer.AddPolicy(role, "/api/v1/organizations/*/import", "POST")
		if err != nil {
			log.Printf("Failed to add policy for role %s on /import: %v", role, err)
		}
	}

	log.Println("✅ Organization custom route permissions configured")
}
