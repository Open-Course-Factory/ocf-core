package terminalController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	authMiddleware "soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"
	paymentMiddleware "soli/formations/src/payment/middleware"
	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"

	"gorm.io/gorm"
)

func TerminalRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	terminalController := NewTerminalController(db)
	middleware := auth.NewAuthMiddleware(db)
	verificationMiddleware := authMiddleware.NewEmailVerificationMiddleware(db)
	usageLimitMiddleware := paymentMiddleware.NewUsageLimitMiddleware(db)
	terminalAccessMiddleware := terminalMiddleware.NewTerminalAccessMiddleware(db)

	routes := router.Group("/terminals")
	// Require email verification for all terminal routes
	routes.Use(verificationMiddleware.RequireVerifiedEmail())

	// Routes spécialisées pour les fonctionnalités Terminal Trainer
	// Apply terminal creation limit middleware to start-session route
	routes.POST("/start-session", middleware.AuthManagement(), usageLimitMiddleware.CheckTerminalCreationLimit(), terminalController.StartSession)

	// Console access requires "read" level access (Layer 2 security check)
	routes.GET("/:id/console", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("read"), terminalController.ConnectConsole)

	// Bulk operations for groups
	groupRoutes := router.Group("/class-groups")
	// NOTE: We don't apply CheckTerminalCreationLimit() here because bulk creation has its own quota logic
	// Instead, we use InjectSubscriptionInfo() to attach the plan to context for validation
	subscriptionMiddleware := paymentMiddleware.NewSubscriptionIntegrationMiddleware(db)
	groupRoutes.POST("/:groupId/bulk-create-terminals", middleware.AuthManagement(), subscriptionMiddleware.InjectSubscriptionInfo(), terminalController.BulkCreateTerminalsForGroup)

	// Stop session requires owner or admin access (Layer 2 security check)
	routes.POST("/:id/stop", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("admin"), terminalController.StopSession)
	routes.GET("/user-sessions", middleware.AuthManagement(), terminalController.GetUserSessions)

	// Routes de partage de terminaux (Layer 2 security checks for terminal owner)
	routes.POST("/:id/share", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("admin"), terminalController.ShareTerminal)
	routes.DELETE("/:id/share/:user_id", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("admin"), terminalController.RevokeTerminalAccess)
	routes.GET("/:id/shares", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("admin"), terminalController.GetTerminalShares)
	routes.GET("/shared-with-me", middleware.AuthManagement(), terminalController.GetSharedTerminals) // No terminal ID, no Layer 2 check
	routes.GET("/:id/info", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("read"), terminalController.GetSharedTerminalInfo)

	// Routes de masquage de terminaux (Layer 2 security checks)
	routes.POST("/:id/hide", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("read"), terminalController.HideTerminal)
	routes.DELETE("/:id/hide", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("read"), terminalController.UnhideTerminal)

	// Sync routes (Layer 2 security checks)
	routes.POST("/:id/sync", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("read"), terminalController.SyncSession)
	routes.POST("/sync-all", middleware.AuthManagement(), terminalController.SyncAllSessions) // No terminal ID, no Layer 2 check
	routes.GET("/:id/status", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("read"), terminalController.GetSessionStatus)
	routes.GET("/:id/access-status", middleware.AuthManagement(), terminalController.GetAccessStatus)

	// Configuration (no terminal-specific access needed)
	routes.GET("/instance-types", middleware.AuthManagement(), terminalController.GetInstanceTypes)
	routes.GET("/metrics", middleware.AuthManagement(), terminalController.GetServerMetrics)

	// Enum service endpoints (admin only - for debugging and diagnostics)
	routes.GET("/enums/status", middleware.AuthManagement(), terminalController.GetEnumStatus)
	routes.POST("/enums/refresh", middleware.AuthManagement(), terminalController.RefreshEnums)

	// Correction des permissions (no terminal-specific access needed)
	routes.POST("/fix-hide-permissions", middleware.AuthManagement(), terminalController.FixTerminalHidePermissions)
}

func UserTerminalKeyRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userTerminalKeyController := NewUserTerminalKeyController(db)
	middleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/user-terminal-keys")

	// Routes spécialisées
	routes.POST("/regenerate", middleware.AuthManagement(), userTerminalKeyController.RegenerateKey)
	routes.GET("/my-key", middleware.AuthManagement(), userTerminalKeyController.GetMyKey)
}
