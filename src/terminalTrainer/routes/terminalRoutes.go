package terminalController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"
	paymentMiddleware "soli/formations/src/payment/middleware"
	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"
	"soli/formations/src/terminalTrainer/models"

	"gorm.io/gorm"
)

func TerminalRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	terminalController := NewTerminalController(db)
	middleware := auth.NewAuthMiddleware(db)
	usageLimitMiddleware := paymentMiddleware.NewUsageLimitMiddleware(db)
	terminalAccessMiddleware := terminalMiddleware.NewTerminalAccessMiddleware(db)

	routes := router.Group("/terminals")

	// Routes spécialisées pour les fonctionnalités Terminal Trainer
	// Apply terminal creation limit middleware to start-session route
	routes.POST("/start-session", middleware.AuthManagement(), usageLimitMiddleware.CheckTerminalCreationLimit(), terminalController.StartSession)

	// Console access requires "read" level access (Layer 2 security check)
	routes.GET("/:id/console", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelRead), terminalController.ConnectConsole)

	// Bulk operations for groups
	groupRoutes := router.Group("/class-groups")
	// NOTE: We don't apply CheckTerminalCreationLimit() here because bulk creation has its own quota logic
	// Instead, we use InjectSubscriptionInfo() to attach the plan to context for validation
	subscriptionMiddleware := paymentMiddleware.NewSubscriptionIntegrationMiddleware(db)
	groupRoutes.POST("/:groupId/bulk-create-terminals", middleware.AuthManagement(), subscriptionMiddleware.InjectSubscriptionInfo(), terminalController.BulkCreateTerminalsForGroup)

	// Stop session requires owner or admin access (Layer 2 security check)
	routes.POST("/:id/stop", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelOwner), terminalController.StopSession)
	routes.GET("/user-sessions", middleware.AuthManagement(), terminalController.GetUserSessions)

	// Routes de partage de terminaux (Layer 2 security checks for terminal owner)
	routes.POST("/:id/share", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelOwner), terminalController.ShareTerminal)
	routes.DELETE("/:id/share/:user_id", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelOwner), terminalController.RevokeTerminalAccess)
	routes.GET("/:id/shares", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelOwner), terminalController.GetTerminalShares)
	routes.GET("/shared-with-me", middleware.AuthManagement(), terminalController.GetSharedTerminals) // No terminal ID, no Layer 2 check
	routes.GET("/:id/info", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelRead), terminalController.GetSharedTerminalInfo)

	// Routes de masquage de terminaux (Layer 2 security checks)
	routes.POST("/:id/hide", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelRead), terminalController.HideTerminal)
	routes.DELETE("/:id/hide", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelRead), terminalController.UnhideTerminal)

	// Sync routes (Layer 2 security checks)
	routes.POST("/:id/sync", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelRead), terminalController.SyncSession)
	routes.POST("/sync-all", middleware.AuthManagement(), terminalController.SyncAllSessions) // No terminal ID, no Layer 2 check
	routes.GET("/:id/status", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelRead), terminalController.GetSessionStatus)
	routes.GET("/:id/access-status", middleware.AuthManagement(), terminalController.GetAccessStatus)

	// Command history routes (no terminal access middleware - handlers verify access internally,
	// and history must remain accessible for expired/stopped sessions)
	routes.DELETE("/my-history", middleware.AuthManagement(), terminalController.DeleteAllUserHistory)
	routes.GET("/:id/history", middleware.AuthManagement(), terminalController.GetSessionHistory)
	routes.DELETE("/:id/history", middleware.AuthManagement(), terminalController.DeleteSessionHistory)

	// Configuration (no terminal-specific access needed)
	routes.GET("/instance-types", middleware.AuthManagement(), terminalController.GetInstanceTypes)
	routes.GET("/metrics", middleware.AuthManagement(), terminalController.GetServerMetrics)
	routes.GET("/backends", middleware.AuthManagement(), terminalController.GetBackends)
	routes.PATCH("/backends/:backendId/set-default", middleware.AuthManagement(), terminalController.SetDefaultBackend)

	// Enum service endpoints (admin only - for debugging and diagnostics)
	routes.GET("/enums/status", middleware.AuthManagement(), terminalController.GetEnumStatus)
	routes.POST("/enums/refresh", middleware.AuthManagement(), terminalController.RefreshEnums)

	// Correction des permissions (no terminal-specific access needed)
	routes.POST("/fix-hide-permissions", middleware.AuthManagement(), terminalController.FixTerminalHidePermissions)

	// Organization terminal sessions (for trainers/managers)
	orgRoutes := router.Group("/organizations")
	orgRoutes.GET("/:orgId/terminal-sessions", middleware.AuthManagement(), terminalController.GetOrganizationTerminalSessions)
}

func UserTerminalKeyRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userTerminalKeyController := NewUserTerminalKeyController(db)
	middleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/user-terminal-keys")

	// Routes spécialisées
	routes.POST("/regenerate", middleware.AuthManagement(), userTerminalKeyController.RegenerateKey)
	routes.GET("/my-key", middleware.AuthManagement(), userTerminalKeyController.GetMyKey)
}
