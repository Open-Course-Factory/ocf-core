package passwordResetRoutes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PasswordResetRoutes registers all password reset routes
func PasswordResetRoutes(router *gin.RouterGroup, db *gorm.DB) {
	controller := NewPasswordResetController(db)

	// Public routes (no authentication required)
	router.POST("/password-reset/request", controller.RequestPasswordReset)
	router.POST("/password-reset/confirm", controller.ResetPassword)
}
