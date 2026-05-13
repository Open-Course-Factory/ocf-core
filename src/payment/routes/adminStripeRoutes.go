package paymentController

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	auth "soli/formations/src/auth"
	"soli/formations/src/payment/services"
)

// adminStripePendingSyncsLimit caps the number of rows returned by the admin
// endpoint. Pending depth should rarely exceed a handful; 200 is plenty of
// headroom and bounds memory if something goes sideways.
const adminStripePendingSyncsLimit = 200

// NewAdminStripePendingSyncsHandler returns the admin endpoint that surfaces
// pending Stripe sync rows for operator visibility. Admin-only; mirrors the
// auth pattern from src/observability/routes/observabilityController.go.
//
// Response JSON: {"count": N, "items": [{id, plan_id, operation, state,
// attempts, last_error, last_attempt_at, created_at}, ...]}
func NewAdminStripePendingSyncsHandler(queue services.StripeSyncQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isAdminFromContext(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "administrator role required"})
			return
		}

		rows, err := queue.ListPending(adminStripePendingSyncsLimit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pending stripe syncs"})
			return
		}

		items := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			items = append(items, map[string]any{
				"id":              r.ID,
				"plan_id":         r.PlanID,
				"operation":       r.Operation,
				"state":           r.State,
				"attempts":        r.Attempts,
				"last_error":      r.LastError,
				"last_attempt_at": r.LastAttemptAt,
				"created_at":      r.CreatedAt,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"count": len(items),
			"items": items,
		})
	}
}

// isAdminFromContext reads the userRoles slice set by upstream auth middleware
// (or by the test router stub) and returns true iff "administrator" is present.
// Local helper to avoid cross-package imports; identical in shape to the
// observability package's isAdmin.
func isAdminFromContext(c *gin.Context) bool {
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

// RegisterAdminStripeRoutes wires the admin Stripe queue endpoint into the
// /admin route group. Layer 1 (RBAC) is enforced via AuthManagement(); Layer 2
// (AdminOnly) is declared in adminStripePermissions.go and enforced by the
// global Layer2Enforcement middleware.
func RegisterAdminStripeRoutes(router *gin.RouterGroup, db *gorm.DB, queue services.StripeSyncQueue) {
	mw := auth.NewAuthMiddleware(db)
	admin := router.Group("/admin")
	admin.GET("/stripe/pending-syncs", mw.AuthManagement(), NewAdminStripePendingSyncsHandler(queue))
}
