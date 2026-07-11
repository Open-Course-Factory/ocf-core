package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// OrganizationRolePlanRoutes mounts the org-scoped role→plan listing. The flat
// /organization-role-plans entity route stays platform-admin-only; this sibling
// gives org managers a server-side, org-scoped read of their own mappings.
func OrganizationRolePlanRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	rolePlanController := NewOrganizationRolePlanController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	// Organization-scoped role→plan listing. Layer 2 (OrgRole, manager+) gates
	// access — see RegisterPaymentPermissions.
	orgRoutes := router.Group("/organizations/:id")
	orgRoutes.Use(authMiddleware.AuthManagement())
	orgRoutes.GET("/role-plans", rolePlanController.GetOrganizationRolePlans)
}
