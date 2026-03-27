package scenarioController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"
	scenarioMiddleware "soli/formations/src/scenarios/middleware"

	"gorm.io/gorm"
)

// ScenarioRoutes registers the custom (non-CRUD) scenario endpoints
func ScenarioRoutes(router *gin.RouterGroup, _ *config.Configuration, db *gorm.DB) {
	controller := NewScenarioController(db)
	middleware := auth.NewAuthMiddleware(db)

	// Scenario management routes (admin/trainer)
	scenarioRoutes := router.Group("/scenarios")
	scenarioRoutes.POST("/import", middleware.AuthManagement(), controller.ImportScenario)
	scenarioRoutes.POST("/seed", middleware.AuthManagement(), controller.SeedScenario)
	scenarioRoutes.POST("/upload", middleware.AuthManagement(), controller.UploadScenario)
	scenarioRoutes.GET("/:id/export", middleware.AuthManagement(), controller.ExportScenario)
	scenarioRoutes.POST("/export", middleware.AuthManagement(), controller.ExportScenarios)
	scenarioRoutes.POST("/import-json", middleware.AuthManagement(), controller.ImportJSON)

	// Session routes (students)
	rateLimiter := scenarioMiddleware.PerUserRateLimit()
	sessionRoutes := router.Group("/scenario-sessions")
	sessionRoutes.GET("/available", middleware.AuthManagement(), controller.GetAvailableScenarios)
	sessionRoutes.GET("/my", middleware.AuthManagement(), controller.GetMySessions)
	sessionRoutes.POST("/start", middleware.AuthManagement(), controller.StartScenario)
	sessionRoutes.GET("/by-terminal/:terminalId", middleware.AuthManagement(), controller.GetSessionByTerminal)
	sessionRoutes.GET("/:id/info", middleware.AuthManagement(), controller.GetSessionInfo)
	sessionRoutes.GET("/:id/flags", middleware.AuthManagement(), controller.GetSessionFlags)
	sessionRoutes.GET("/:id/current-step", middleware.AuthManagement(), controller.GetCurrentStep)
	sessionRoutes.GET("/:id/step/:stepOrder", middleware.AuthManagement(), controller.GetStepByOrder)
	sessionRoutes.POST("/:id/verify", middleware.AuthManagement(), rateLimiter, controller.VerifyStep)
	sessionRoutes.POST("/:id/submit-flag", middleware.AuthManagement(), rateLimiter, controller.SubmitFlag)
	sessionRoutes.POST("/:id/steps/:stepOrder/hints/:level/reveal", middleware.AuthManagement(), controller.RevealHint)
	sessionRoutes.POST("/:id/abandon", middleware.AuthManagement(), controller.AbandonSession)

	// Group-level scenario import/export routes (teachers/group admins)
	groupScenarioRoutes := router.Group("/groups/:groupId/scenarios")
	groupScenarioRoutes.GET("/:scenarioId/export", middleware.AuthManagement(), controller.GroupExportScenario)
	groupScenarioRoutes.POST("/import-json", middleware.AuthManagement(), controller.GroupImportJSON)
	groupScenarioRoutes.POST("/upload", middleware.AuthManagement(), controller.GroupUploadScenario)

	// ProjectFile custom routes
	projectFileCtrl := NewProjectFileController(db)
	projectFileRoutes := router.Group("/project-files")
	projectFileRoutes.GET("/:id/content", middleware.AuthManagement(), projectFileCtrl.GetContent)

	// Teacher dashboard routes
	teacherCtrl := NewTeacherController(db)
	teacherRoutes := router.Group("/teacher")
	teacherRoutes.GET("/groups/:groupId/activity", middleware.AuthManagement(), teacherCtrl.GetGroupActivity)
	teacherRoutes.GET("/groups/:groupId/scenarios/:scenarioId/results", middleware.AuthManagement(), teacherCtrl.GetScenarioResults)
	teacherRoutes.GET("/groups/:groupId/scenarios/:scenarioId/analytics", middleware.AuthManagement(), teacherCtrl.GetScenarioAnalytics)
	teacherRoutes.GET("/groups/:groupId/sessions/:sessionId/detail", middleware.AuthManagement(), teacherCtrl.GetSessionDetail)
	teacherRoutes.POST("/groups/:groupId/scenarios/:scenarioId/bulk-start", middleware.AuthManagement(), teacherCtrl.BulkStartScenario)
	teacherRoutes.POST("/groups/:groupId/scenarios/:scenarioId/reset-sessions", middleware.AuthManagement(), teacherCtrl.ResetGroupScenarioSessions)
}
