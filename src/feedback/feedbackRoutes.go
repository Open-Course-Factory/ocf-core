package feedback

import (
	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FeedbackRoutes registers the feedback endpoints
func FeedbackRoutes(router *gin.RouterGroup, _ *config.Configuration, db *gorm.DB) {
	controller := NewFeedbackController(db)
	middleware := auth.NewAuthMiddleware(db)

	feedbackGroup := router.Group("/feedback")
	feedbackGroup.Use(middleware.AuthManagement())
	{
		feedbackGroup.POST("/send", controller.SendFeedback)
	}
}
