package adminUsersRoutes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

// RegisterRoutes wires the admin users-with-memberships listing
// onto the given router group. Layer 1 (RBAC) is enforced via
// AuthManagement(); Layer 2 (admin-only) is declared in permissions.go
// and enforced by the global Layer2Enforcement middleware.
func RegisterRoutes(router *gin.RouterGroup, db *gorm.DB) {
	mw := auth.NewAuthMiddleware(db)

	admin := router.Group("/admin")
	admin.GET("/users-with-memberships", mw.AuthManagement(), NewListUsersHandler(db))
}
