package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/services"
)

// ImpersonationHeader is the HTTP header an admin sets to request that the
// downstream handlers see the request as if it were performed by the target
// user. The middleware only swaps the gin context identity if the caller has
// an active, fresh impersonation session for that exact target.
const ImpersonationHeader = "X-Impersonate-User"

// RolesResolver resolves the platform roles for a given user ID. The
// middleware uses it to populate "userRoles" on the gin context after
// swapping the identity to the impersonation target. Implementations
// typically wrap a Casdoor lookup or the local Casbin grouping table.
type RolesResolver func(userID string) ([]string, error)

// ImpersonationMiddleware swaps the gin context identity from the calling
// admin to the impersonation target when the request carries a valid
// X-Impersonate-User header.
//
// Pre-conditions: an upstream auth middleware (e.g. AuthManagement) must have
// populated "userId" and "userRoles" on the context for authenticated
// requests. When the impersonation header is absent, the middleware is a
// no-op so non-impersonated traffic is unaffected.
//
// On success the middleware:
//   - stores the original caller ID in "impersonatorId"
//   - stores the original caller roles in "impersonatorRoles"
//   - swaps "userId" to the target ID
//   - swaps "userRoles" to the target's roles (resolved via resolveRoles)
//   - calls Touch on the active session (best effort) to bump idle activity
//
// On failure the request is aborted with a JSON `{"error": "..."}` body using
// one of the documented error codes (unauthenticated, impersonation_forbidden,
// impersonation_invalid, impersonation_expired, impersonation_role_lookup_failed).
func ImpersonationMiddleware(svc services.ImpersonationService, resolveRoles RolesResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetID := c.GetHeader(ImpersonationHeader)
		if targetID == "" {
			c.Next()
			return
		}

		// 1. The caller must be authenticated (AuthManagement should have run).
		callerID := c.GetString("userId")
		if callerID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		// 2. The caller must be a platform admin.
		callerRolesRaw, _ := c.Get("userRoles")
		callerRoles, _ := callerRolesRaw.([]string)
		if !access.IsAdmin(callerRoles) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "impersonation_forbidden"})
			return
		}

		// 3. There must be an active impersonation session AND it must target
		//    the user named in the header.
		session, err := svc.GetActiveSession(callerID)
		if err != nil || session == nil || session.TargetID != targetID {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "impersonation_invalid"})
			return
		}

		// 4. Reject (and close) sessions that have gone idle past the timeout.
		if time.Since(session.LastActivityAt) > services.ImpersonationIdleTimeout {
			_ = svc.StopSession(callerID, "expired")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "impersonation_expired"})
			return
		}

		// 5. Resolve the target's roles (needed by downstream auth checks).
		targetRoles, err := resolveRoles(targetID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "impersonation_role_lookup_failed"})
			return
		}

		// 6. Best-effort activity bump. Failing to touch must not break the
		//    request — the next request will re-attempt and ExpireStale is
		//    the ultimate safety net.
		_ = svc.Touch(session.ID)

		// 7. Swap the identity: stash the impersonator's original values, then
		//    overwrite userId/userRoles with the target's.
		c.Set("impersonatorId", callerID)
		c.Set("impersonatorRoles", callerRoles)
		c.Set("userId", targetID)
		c.Set("userRoles", targetRoles)

		c.Next()
	}
}
