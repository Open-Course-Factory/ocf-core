package routes

import (
	"log"

	auth "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"
	config "soli/formations/src/configuration"
	"soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/services"
	"soli/formations/src/utils"

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

		// Convert personal organization to team organization
		organizations.POST("/:id/convert-to-team", middleware.AuthManagement(), orgController.ConvertToTeam)

		// Backend assignment management
		organizations.GET("/:id/backends", middleware.AuthManagement(), orgController.GetOrganizationBackends)
		organizations.PUT("/:id/backends", middleware.AuthManagement(), orgController.UpdateOrganizationBackends)
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
	opts := utils.PermissionOptions{WarnOnError: true}
	for _, role := range roles {
		if err := utils.AddPolicy(casdoor.Enforcer, role, "/api/v1/organizations/*/members", "GET", opts); err != nil {
			log.Printf("Failed to add policy for role %s on /members: %v", role, err)
		}
	}

	// Add permissions for /groups endpoint (read-only)
	for _, role := range roles {
		if err := utils.AddPolicy(casdoor.Enforcer, role, "/api/v1/organizations/*/groups", "GET", opts); err != nil {
			log.Printf("Failed to add policy for role %s on /groups: %v", role, err)
		}
	}

	// Add permissions for /import endpoint (admin/supervisor only)
	adminRoles := []string{"administrator", "supervisor", "admin", "trainer"}
	for _, role := range adminRoles {
		if err := utils.AddPolicy(casdoor.Enforcer, role, "/api/v1/organizations/*/import", "POST", opts); err != nil {
			log.Printf("Failed to add policy for role %s on /import: %v", role, err)
		}
	}

	// Add permissions for /convert-to-team endpoint (all authenticated users - owner check in handler)
	for _, role := range roles {
		if err := utils.AddPolicy(casdoor.Enforcer, role, "/api/v1/organizations/*/convert-to-team", "POST", opts); err != nil {
			log.Printf("Failed to add policy for role %s on /convert-to-team: %v", role, err)
		}
	}

	// Add permissions for /backends endpoint (read: all roles, write: admin only)
	for _, role := range roles {
		if err := utils.AddPolicy(casdoor.Enforcer, role, "/api/v1/organizations/*/backends", "GET", opts); err != nil {
			log.Printf("Failed to add policy for role %s on /backends GET: %v", role, err)
		}
	}
	adminBackendRoles := []string{"administrator", "admin"}
	for _, role := range adminBackendRoles {
		if err := utils.AddPolicy(casdoor.Enforcer, role, "/api/v1/organizations/*/backends", "PUT", opts); err != nil {
			log.Printf("Failed to add policy for role %s on /backends PUT: %v", role, err)
		}
	}

	log.Println("✅ Organization custom route permissions configured")
}
