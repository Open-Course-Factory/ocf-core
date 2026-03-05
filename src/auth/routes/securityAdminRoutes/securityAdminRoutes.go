package securityAdminRoutes

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"

	"gorm.io/gorm"
)

func SecurityAdminRoutes(router *gin.RouterGroup, db *gorm.DB) {
	controller := NewSecurityAdminController(casdoor.Enforcer, db)

	routes := router.Group("/admin/security")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("/policies", middleware.AuthManagement(), controller.GetPolicyOverview)
	routes.GET("/user-permissions", middleware.AuthManagement(), controller.GetUserPermissionLookup)
	routes.GET("/entity-roles", middleware.AuthManagement(), controller.GetEntityRoleMatrix)
	routes.GET("/health-checks", middleware.AuthManagement(), controller.GetPolicyHealthChecks)
}
