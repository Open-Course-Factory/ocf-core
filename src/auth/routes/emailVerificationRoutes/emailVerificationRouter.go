package emailVerificationRoutes

import (
	auth "soli/formations/src/auth"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func EmailVerificationRoutes(router *gin.RouterGroup, db *gorm.DB) {
	controller := NewEmailVerificationController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	// Public routes
	router.POST("/verify-email", controller.VerifyEmail)
	router.POST("/resend-verification", controller.ResendVerification)

	// Protected route
	router.GET("/verify-status", authMiddleware.AuthManagement(), controller.GetVerificationStatus)
}
