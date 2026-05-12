package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/observability"
)

// NewObservabilityHandler returns the admin-only /admin/observability-metrics
// handler. The handler reads userRoles from gin context (set by AuthManagement
// upstream in production, by the test router stub in tests) and returns 403
// unless the caller is administrator.
func NewObservabilityHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "administrator role required"})
			return
		}

		c.JSON(http.StatusOK, snapshot())
	}
}

// isAdmin reads the userRoles slice set by upstream auth middleware (or by the
// test router stub) and returns true iff "administrator" is present.
func isAdmin(c *gin.Context) bool {
	rolesAny, exists := c.Get("userRoles")
	if !exists {
		return false
	}
	roles, ok := rolesAny.([]string)
	if !ok {
		return false
	}
	for _, r := range roles {
		if r == "administrator" {
			return true
		}
	}
	return false
}

// snapshot builds the JSON-ready response. Pure function — easy to test.
// GetRecentErrors returns nil for empty buffer; coerce to [] for JSON so the
// contract is "always an array, never null" (locked by the zero-snapshot test).
func snapshot() map[string]any {
	m := observability.Metrics

	recent := hooks.GlobalHookRegistry.GetRecentErrors(0)
	if recent == nil {
		recent = make([]hooks.HookError, 0)
	}

	return map[string]any{
		"stripe": map[string]any{
			"create":  map[string]uint64{"success": m.StripeCreateSuccess.Load(), "failure": m.StripeCreateFailure.Load()},
			"update":  map[string]uint64{"success": m.StripeUpdateSuccess.Load(), "failure": m.StripeUpdateFailure.Load()},
			"archive": map[string]uint64{"success": m.StripeArchiveSuccess.Load(), "failure": m.StripeArchiveFailure.Load()},
			"panics":  m.StripeSyncPanic.Load(),
		},
		"scenarios": map[string]any{
			"setup_panics":             m.ScenarioSetupPanic.Load(),
			"setup_failed_transitions": m.ScenarioSetupFailed.Load(),
			"terminal_stop_failures":   m.TerminalStopOnCleanupFailure.Load(),
		},
		"hooks": map[string]any{
			"recent_errors": recent,
		},
	}
}
