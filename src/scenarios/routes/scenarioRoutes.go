package scenarioController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// ScenarioRoutes registers the custom (non-CRUD) scenario endpoints
func ScenarioRoutes(router *gin.RouterGroup, _ *config.Configuration, db *gorm.DB) {
	controller := NewScenarioController(db)
	middleware := auth.NewAuthMiddleware(db)

	// Scenario management routes (admin/trainer)
	scenarioRoutes := router.Group("/scenarios")
	scenarioRoutes.POST("/import", middleware.AuthManagement(), controller.ImportScenario)
	scenarioRoutes.POST("/:id/start", middleware.AuthManagement(), controller.StartScenario)

	// Session routes (students)
	sessionRoutes := router.Group("/scenario-sessions")
	sessionRoutes.GET("/by-terminal/:terminalId", middleware.AuthManagement(), controller.GetSessionByTerminal)
	sessionRoutes.GET("/:id/current-step", middleware.AuthManagement(), controller.GetCurrentStep)
	sessionRoutes.POST("/:id/verify", middleware.AuthManagement(), controller.VerifyStep)
	sessionRoutes.POST("/:id/submit-flag", middleware.AuthManagement(), controller.SubmitFlag)
	sessionRoutes.POST("/:id/abandon", middleware.AuthManagement(), controller.AbandonSession)
}
