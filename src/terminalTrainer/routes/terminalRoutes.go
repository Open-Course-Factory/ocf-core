package terminalController

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"
	configRepositories "soli/formations/src/configuration/repositories"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentServices "soli/formations/src/payment/services"
	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"
	"soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

func TerminalRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	terminalController := NewTerminalController(db)
	middleware := auth.NewAuthMiddleware(db)
	effectivePlanService := paymentServices.NewEffectivePlanService(db)
	terminalService := terminalServices.NewTerminalTrainerService(db)
	terminalAccessMiddleware := terminalMiddleware.NewTerminalAccessMiddleware(db)

	routes := router.Group("/terminals")

	// Console access requires "read" level access (Layer 2 security check)
	routes.GET("/:id/console", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(models.AccessLevelRead), terminalController.ConnectConsole)

	// Bulk operations for groups
	groupRoutes := router.Group("/class-groups")
	// Use effective plan middlewares for bulk creation validation
	groupRoutes.POST("/:id/bulk-create-terminals", middleware.AuthManagement(), paymentMiddleware.InjectEffectivePlan(effectivePlanService), paymentMiddleware.RequirePlan(), terminalController.BulkCreateTerminalsForGroup)
	groupRoutes.GET("/:id/command-history", middleware.AuthManagement(), terminalController.GetGroupCommandHistory)
	groupRoutes.GET("/:id/command-history-stats", middleware.AuthManagement(), terminalController.GetGroupCommandHistoryStats)

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

	// Consent status (checks org/group-level consent policy)
	routes.GET("/consent-status", middleware.AuthManagement(), terminalController.GetConsentStatus)

	// Configuration (no terminal-specific access needed)
	routes.GET("/metrics", middleware.AuthManagement(), terminalController.GetServerMetrics)
	routes.GET("/backends", middleware.AuthManagement(), terminalController.GetBackends)
	routes.PATCH("/backends/:backendId/set-default", middleware.AuthManagement(), terminalController.SetDefaultBackend)

	// Enum service endpoints (admin only - for debugging and diagnostics)
	routes.GET("/enums/status", middleware.AuthManagement(), terminalController.GetEnumStatus)
	routes.POST("/enums/refresh", middleware.AuthManagement(), terminalController.RefreshEnums)

	// Composed session routes (Phase 4)
	routes.GET("/distributions", middleware.AuthManagement(), terminalController.GetDistributions)
	routes.GET("/session-options", middleware.AuthManagement(), paymentMiddleware.InjectOrgContext(), paymentMiddleware.InjectEffectivePlan(effectivePlanService), paymentMiddleware.RequirePlan(), terminalController.GetSessionOptions)
	routes.POST("/start-composed-session", middleware.AuthManagement(), paymentMiddleware.InjectOrgContext(), paymentMiddleware.InjectEffectivePlan(effectivePlanService), paymentMiddleware.RequirePlan(), paymentMiddleware.CheckLimit(effectivePlanService, db, "concurrent_terminals"), paymentMiddleware.CheckRAMAvailability(terminalService), terminalController.StartComposedSession)

	// Correction des permissions (no terminal-specific access needed)
	routes.POST("/fix-hide-permissions", middleware.AuthManagement(), terminalController.FixTerminalHidePermissions)

	// Organization terminal sessions (for trainers/managers)
	orgRoutes := router.Group("/organizations")
	orgRoutes.GET("/:id/terminal-sessions", middleware.AuthManagement(), terminalController.GetOrganizationTerminalSessions)

	// Incus UI reverse proxy (admin + org owner/manager only)
	// The cookie-to-header middleware extracts the JWT from the incus_token
	// cookie (set by the frontend iframe loader) and injects it as an
	// Authorization header so the standard auth middleware can validate it.
	// Build set of protected backends (admin-only for Incus UI proxy).
	// Includes the system default backend (from tt-backend) + any listed in incus_ui_protected_backends.
	protectedBackends := make(map[string]bool)
	// Get system default backend from tt-backend (single source of truth)
	if backends, err := terminalService.GetBackends(); err == nil {
		for _, b := range backends {
			if b.IsDefault {
				protectedBackends[b.ID] = true
				break
			}
		}
	}
	featureRepo := configRepositories.NewFeatureRepository(db)
	if f, err := featureRepo.GetFeatureByKey("incus_ui_protected_backends"); err == nil && f.Value != "" {
		for _, id := range strings.Split(f.Value, ",") {
			if id = strings.TrimSpace(id); id != "" {
				protectedBackends[id] = true
			}
		}
	}
	incusUIController := NewIncusUIController(db, os.Getenv("TERMINAL_TRAINER_URL"), protectedBackends, os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"))
	incusUIRoutes := router.Group("/incus-ui")
	incusUIRoutes.Any("/:backendId/*path", incusCookieAuth(), middleware.AuthManagement(), incusUIController.ProxyIncusUI)
}

func UserTerminalKeyRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userTerminalKeyController := NewUserTerminalKeyController(db)
	middleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/user-terminal-keys")

	// Routes spécialisées
	routes.POST("/regenerate", middleware.AuthManagement(), userTerminalKeyController.RegenerateKey)
	routes.GET("/my-key", middleware.AuthManagement(), userTerminalKeyController.GetMyKey)
}
