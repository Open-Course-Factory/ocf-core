package terminalController

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	access "soli/formations/src/auth/access"
	config "soli/formations/src/configuration"
	configRepositories "soli/formations/src/configuration/repositories"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	paymentMiddleware "soli/formations/src/payment/middleware"
	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

func TerminalRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	terminalController := NewTerminalController(db)
	middleware := auth.NewAuthMiddleware(db)
	terminalService := terminalServices.NewTerminalTrainerService(db)
	terminalAccessMiddleware := terminalMiddleware.NewTerminalAccessMiddleware(db)

	routes := router.Group("/terminals")

	// Console access requires terminal ownership (Layer 2 security check)
	routes.GET("/:id/console", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(), terminalController.ConnectConsole)

	// Bulk operations for groups
	groupRoutes := router.Group("/class-groups")
	// Use effective plan middlewares for bulk creation validation.
	groupRoutes.POST("/:id/bulk-create-terminals", paymentMiddleware.WithPlanChain(
		db, entityManagementInterfaces.PlanRequirement{RequirePlan: true}, terminalService,
		[]gin.HandlerFunc{middleware.AuthManagement()},
		terminalController.BulkCreateTerminalsForGroup,
	)...)
	groupRoutes.GET("/:id/command-history", middleware.AuthManagement(), terminalController.GetGroupCommandHistory)
	groupRoutes.GET("/:id/command-history-stats", middleware.AuthManagement(), terminalController.GetGroupCommandHistoryStats)

	// Stop session requires terminal ownership (Layer 2 security check)
	routes.POST("/:id/stop", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess(), terminalController.StopSession)
	// Resume a stopped session — slot-neutral state transition. The
	// resumed session may consume paid features so the plan-validity gate
	// applies, but NO capacity check is needed: the budget engine already
	// counts the persistent session against the user's CPU/RAM cap at
	// creation (D6': a stopped session occupies a slot), and tt-backend's
	// resume handler is a state transition on an existing container — not
	// a fresh allocation.
	//
	// What stays here:
	//   - RequireTerminalAccessAllowStopped: Layer 2 ownership.
	//   - InjectOrgContext / InjectEffectivePlan / RequirePlan: the resumed
	//     session may consume paid features; the plan-validity gate still
	//     applies.
	//
	// What was removed (regression: spurious 503 on every Resume):
	//   - CheckRAMAvailability — Resume sends no body, so the middleware
	//     fell back to a plan-max (LargestSize) estimate. On any host whose
	//     headroom was below that estimate (i.e. realistic production
	//     state) every Resume 503'd, even though the session's actual
	//     footprint had not changed since it was admitted at creation.
	routes.POST("/:id/start", paymentMiddleware.WithPlanChain(
		db, entityManagementInterfaces.PlanRequirement{OrgContext: true, RequirePlan: true}, terminalService,
		[]gin.HandlerFunc{middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccessAllowStopped()},
		terminalController.StartSession,
	)...)
	// Permanently delete a session — ownership enforced via Layer 2,
	// StateStopped allowed.
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
	routes.GET("/session-options", paymentMiddleware.WithPlanChain(
		db, entityManagementInterfaces.PlanRequirement{OrgContext: true, RequirePlan: true}, terminalService,
		[]gin.HandlerFunc{middleware.AuthManagement()},
		terminalController.GetSessionOptions,
	)...)
	// Budget enforcement (CPU/RAM cap from MaxCPU/MaxMemoryMB) is performed
	// inside StartComposedSession via QuotaService.CheckBudget; no
	// middleware-level slot counter is needed. CheckHostRAM verifies host
	// headroom for the chosen size before the handler runs.
	routes.POST("/start-composed-session", paymentMiddleware.WithPlanChain(
		db, entityManagementInterfaces.PlanRequirement{OrgContext: true, RequirePlan: true, CheckHostRAM: true}, terminalService,
		[]gin.HandlerFunc{middleware.AuthManagement()},
		terminalController.StartComposedSession,
	)...)
	// Capacity check: same plan-resolution chain as start-composed-session
	// but no CheckHostRAM — this endpoint IS the check.
	routes.GET("/capacity-check", paymentMiddleware.WithPlanChain(
		db, entityManagementInterfaces.PlanRequirement{OrgContext: true, RequirePlan: true}, terminalService,
		[]gin.HandlerFunc{middleware.AuthManagement()},
		terminalController.CapacityCheck,
	)...)
	// My usage snapshot — read-only personal-or-org view used by the dashboard
	// "Utilisation Actuelle" panel. Same middleware chain as session-options:
	// InjectOrgContext lets the handler read ?organization_id from context;
	// InjectEffectivePlan + RequirePlan ensure an active plan resolves before
	// the handler runs.
	routes.GET("/my-usage", paymentMiddleware.WithPlanChain(
		db, entityManagementInterfaces.PlanRequirement{OrgContext: true, RequirePlan: true}, terminalService,
		[]gin.HandlerFunc{middleware.AuthManagement()},
		terminalController.MyTerminalUsage,
	)...)

	// Organization terminal sessions (for trainers/managers)
	orgRoutes := router.Group("/organizations")
	orgRoutes.GET("/:id/terminal-sessions", middleware.AuthManagement(), terminalController.GetOrganizationTerminalSessions)
	orgRoutes.GET("/:id/terminal-usage", middleware.AuthManagement(), terminalController.GetOrgTerminalUsage)
	orgRoutes.GET("/:id/usage-export", middleware.AuthManagement(), terminalController.GetOrgUsageExport)

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

	// Layer 2 backstop for the Incus UI proxy: share the handler's predicate so
	// the declarative rule and ProxyIncusUI enforce the same policy (admin bypass
	// lives inside IsUserAuthorizedForBackend).
	access.RegisterAccessEnforcer(IncusBackendAccess, func(ctx *gin.Context, rule access.AccessRule, userID string, roles []string) bool {
		backendID := ctx.Param(rule.Param)
		if incusUIController.IsUserAuthorizedForBackend(userID, roles, backendID) {
			return true
		}
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":  "Access denied",
			"detail": "You are not authorized to access this backend",
		})
		return false
	})

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
