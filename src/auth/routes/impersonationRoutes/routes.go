package impersonationRoutes

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	services "soli/formations/src/auth/services"

	"gorm.io/gorm"
)

// ImpersonationRoutes registers the three platform-admin impersonation
// endpoints on the given router group.
//
// The routes are:
//   - POST /admin/impersonate/start  — open a new session (admin only)
//   - POST /admin/impersonate/stop   — close the active session
//   - GET  /admin/impersonate/active — describe the caller's active session
//
// Layer 1 (Casbin RBAC) is enforced via AuthManagement(); Layer 2 (admin-only
// access) is declared in permissions.go and enforced by the global
// Layer2Enforcement middleware.
func ImpersonationRoutes(router *gin.RouterGroup, db *gorm.DB, svc services.ImpersonationService, validator UserValidator) {
	ctrl := NewController(svc, validator)
	mw := auth.NewAuthMiddleware(db)

	admin := router.Group("/admin/impersonate")
	admin.POST("/start", mw.AuthManagement(), ctrl.StartImpersonation)
	admin.POST("/stop", mw.AuthManagement(), ctrl.StopImpersonation)
	admin.GET("/active", mw.AuthManagement(), ctrl.GetActiveImpersonation)
}
