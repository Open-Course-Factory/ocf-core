package routes

import (
	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EmailTemplateRoutes sets up email template management routes
// Note: Standard CRUD routes are handled by the generic entity system at /email-templates
// This function only sets up the custom test email endpoint
func EmailTemplateRoutes(router *gin.RouterGroup, conf *config.Configuration, db *gorm.DB) {
	controller := NewTemplateController(db)
	middleware := auth.NewAuthMiddleware(db)

	// Custom route for testing email templates (not part of standard CRUD)
	admin := router.Group("/email-templates")
	admin.Use(middleware.AuthManagement())
	{
		admin.POST("/:id/test", controller.SendTestEmail)
	}
}
