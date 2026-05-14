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

	// Console access requires terminal ownership (Layer 2 security check)
	routes.GET("/:id/console", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(), terminalController.ConnectConsole)

	// Bulk operations for groups
	groupRoutes := router.Group("/class-groups")
	// Use effective plan middlewares for bulk creation validation
	groupRoutes.POST("/:id/bulk-create-terminals", middleware.AuthManagement(), paymentMiddleware.InjectEffectivePlan(effectivePlanService, db), paymentMiddleware.RequirePlan(), terminalController.BulkCreateTerminalsForGroup)
	groupRoutes.GET("/:id/command-history", middleware.AuthManagement(), terminalController.GetGroupCommandHistory)
	groupRoutes.GET("/:id/command-history-stats", middleware.AuthManagement(), terminalController.GetGroupCommandHistoryStats)

	// Stop session requires terminal ownership (Layer 2 security check)
	routes.POST("/:id/stop", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(), terminalController.StopSession)
	// Resume a stopped session — slot-neutral state transition. The stopped
	// session already counts against `OccupiesSlotScope` (terminals.state
	// IN ('running','stopped') AND expires_at > now), so this path does NOT
	// add a slot and must NOT carry CheckLimit("concurrent_terminals") —
	// doing so 403s the user out of resuming their own session at 1/1.
	// The fresh-create flow at POST /start-composed-session is the only
	// path that adds a slot and remains gated there.
	//
	// What stays here:
	//   - RequireTerminalAccessAllowStopped: Layer 2 ownership.
	//   - InjectOrgContext / InjectEffectivePlan / RequirePlan: the resumed
	//     session may consume paid features; the plan-validity gate still
	//     applies.
	//   - CheckRAMAvailability: RAM is a separate, independent gate (resume
	//     spins the container back up; the host needs the capacity).
	routes.POST("/:id/start",
		middleware.AuthManagement(),
		terminalAccessMiddleware.RequireTerminalAccessAllowStopped(),
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(effectivePlanService, db),
		paymentMiddleware.RequirePlan(),
		paymentMiddleware.CheckRAMAvailability(terminalService),
		terminalController.StartSession,
	)
	// Permanently delete a session — ownership enforced via Layer 2,
	// "stopped" state allowed.
	routes.DELETE("/:id", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccessAllowStopped(), terminalController.DeleteSession)
	routes.GET("/user-sessions", middleware.AuthManagement(), terminalController.GetUserSessions)

	// Sync routes (Layer 2 security checks)
	routes.POST("/:id/sync", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(), terminalController.SyncSession)
	routes.POST("/sync-all", middleware.AuthManagement(), terminalController.SyncAllSessions) // No terminal ID, no Layer 2 check
	routes.GET("/:id/status", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(), terminalController.GetSessionStatus)
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
	routes.GET("/sizes", middleware.AuthManagement(), terminalController.GetSizes)
	routes.GET("/catalog-sizes", middleware.AuthManagement(), terminalController.GetCatalogSizes)
	routes.GET("/catalog-features", middleware.AuthManagement(), terminalController.GetCatalogFeatures)
	routes.GET("/session-options", middleware.AuthManagement(), paymentMiddleware.InjectOrgContext(), paymentMiddleware.InjectEffectivePlan(effectivePlanService, db), paymentMiddleware.RequirePlan(), terminalController.GetSessionOptions)
	routes.POST("/start-composed-session", middleware.AuthManagement(), paymentMiddleware.InjectOrgContext(), paymentMiddleware.InjectEffectivePlan(effectivePlanService, db), paymentMiddleware.RequirePlan(), paymentMiddleware.CheckLimit(effectivePlanService, db, "concurrent_terminals"), paymentMiddleware.CheckRAMAvailability(terminalService), terminalController.StartComposedSession)
	// Capacity check: same plan-resolution chain as start-composed-session
	// but no CheckLimit/CheckRAMAvailability — this endpoint IS the check.
	routes.GET("/capacity-check", middleware.AuthManagement(), paymentMiddleware.InjectOrgContext(), paymentMiddleware.InjectEffectivePlan(effectivePlanService, db), paymentMiddleware.RequirePlan(), terminalController.CapacityCheck)

	// Organization terminal sessions (for trainers/managers)
	orgRoutes := router.Group("/organizations")
	orgRoutes.GET("/:id/terminal-sessions", middleware.AuthManagement(), terminalController.GetOrganizationTerminalSessions)
	orgRoutes.GET("/:id/terminal-usage", middleware.AuthManagement(), terminalController.GetOrgTerminalUsage)

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
