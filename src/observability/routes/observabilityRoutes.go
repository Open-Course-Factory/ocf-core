package routes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

// RegisterRoutes wires the admin-only observability metrics endpoint. Mirrors
// the pattern in src/admin/routes/adminUsersRoutes/routes.go: Layer 1 (RBAC)
// is enforced via AuthManagement(); Layer 2 (admin-only) is declared in
// permissions.go and enforced by the global Layer2Enforcement middleware.
func RegisterRoutes(router *gin.RouterGroup, db *gorm.DB) {
	mw := auth.NewAuthMiddleware(db)

	admin := router.Group("/admin")
	admin.GET("/observability-metrics", mw.AuthManagement(), NewObservabilityHandler())
}
