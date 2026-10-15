package adminUsersRoutes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	auth "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"
)

// RegisterRoutes wires the admin users-with-memberships listing and the
// per-user management actions (password reset, admin role, account block)
// onto the given router group. Layer 1 (RBAC) is enforced via
// AuthManagement(); Layer 2 (admin-only) is declared in permissions.go
// and enforced by the global Layer2Enforcement middleware.
func RegisterRoutes(router *gin.RouterGroup, db *gorm.DB) {
	mw := auth.NewAuthMiddleware(db)

	admin := router.Group("/admin")
	admin.GET("/users-with-memberships", mw.AuthManagement(), NewListUsersHandler(db))
	admin.POST("/users", mw.AuthManagement(), NewCreateUserHandler(casdoor.Enforcer, db))

	users := admin.Group("/users/:id")
	users.POST("/send-password-reset-email", mw.AuthManagement(), NewSendPasswordResetHandler(db))
	users.POST("/admin-role", mw.AuthManagement(), NewGrantAdminRoleHandler(casdoor.Enforcer, db))
	users.DELETE("/admin-role", mw.AuthManagement(), NewRevokeAdminRoleHandler(casdoor.Enforcer, db))
	users.PUT("/forbidden", mw.AuthManagement(), NewSetForbiddenHandler(db))
	users.POST("/verify-email", mw.AuthManagement(), NewVerifyEmailHandler(db))
	users.PUT("/password", mw.AuthManagement(), NewSetPasswordHandler(db))
}
