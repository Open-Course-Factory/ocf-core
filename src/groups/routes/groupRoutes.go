package routes

import (
	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GroupRoutes registers custom (non-CRUD) group endpoints. CRUD is handled by
// the entityManagement framework.
func GroupRoutes(rg *gin.RouterGroup, _ *config.Configuration, db *gorm.DB) {
	groupCtrl := NewGroupController(db)
	middleware := auth.NewAuthMiddleware(db)

	groups := rg.Group("/groups")
	{
		// List the authenticated user's group memberships (per-group role lookup).
		// Registered before any /:id/* routes so "me" is matched as a literal segment.
		groups.GET("/me/memberships", middleware.AuthManagement(), groupCtrl.GetMyGroupMemberships)
	}
}
