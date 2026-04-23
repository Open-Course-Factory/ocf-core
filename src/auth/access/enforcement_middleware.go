package access

import (
	"net/http"
	"sync"

	"soli/formations/src/auth/casdoor"

	"github.com/gin-gonic/gin"
)

// userResolver is the package-level seam used by Layer2Enforcement to
// resolve the authenticated user's ID and roles directly from the
// request's JWT. It is abstracted as a variable so tests can inject a
// stub without needing a live Casdoor configuration.
//
// Production code should not override this — set JwtTokenParser in the
// casdoor package instead if you need to stub JWT parsing itself.
var userResolver = func(ctx *gin.Context) (userID string, roles []string, err error) {
	return casdoor.ResolveUserFromRequest(ctx)
}

// EntityLoader retrieves ownership fields from entity instances.
type EntityLoader interface {
	GetOwnerField(entityName string, entityID string, fieldName string) (string, error)
}

// MembershipChecker verifies a user's role within a group or organization.
type MembershipChecker interface {
	CheckGroupRole(groupID string, userID string, minRole string) (bool, error)
	CheckOrgRole(orgID string, userID string, minRole string) (bool, error)
}

// AccessEnforcer is a function that enforces a specific AccessRuleType.
// It receives the Gin context, the access rule, and the user's ID/roles.
// Returns true if access is granted, false if denied.
// If it returns false, it must call ctx.AbortWithStatusJSON before returning.
type AccessEnforcer func(ctx *gin.Context, rule AccessRule, userID string, roles []string) bool

// enforcerRegistry maps AccessRuleType to its enforcement handler.
// Plugins can register new access rule types or override existing ones.
var enforcerRegistry = struct {
	mu       sync.RWMutex
	handlers map[AccessRuleType]AccessEnforcer
}{
	handlers: make(map[AccessRuleType]AccessEnforcer),
}

// RegisterAccessEnforcer registers a handler for a given AccessRuleType.
// Call this at startup to add custom access rules or override built-in ones.
func RegisterAccessEnforcer(ruleType AccessRuleType, handler AccessEnforcer) {
	enforcerRegistry.mu.Lock()
	defer enforcerRegistry.mu.Unlock()
	enforcerRegistry.handlers[ruleType] = handler
}

// getAccessEnforcer returns the handler for a rule type, or nil if none registered.
func getAccessEnforcer(ruleType AccessRuleType) AccessEnforcer {
	enforcerRegistry.mu.RLock()
	defer enforcerRegistry.mu.RUnlock()
	return enforcerRegistry.handlers[ruleType]
}

// ResetEnforcers clears all registered access enforcers (for testing).
func ResetEnforcers() {
	enforcerRegistry.mu.Lock()
	defer enforcerRegistry.mu.Unlock()
	enforcerRegistry.handlers = make(map[AccessRuleType]AccessEnforcer)
}

// SetUserResolver installs a custom user-resolver function for Layer 2
// enforcement. Intended for use in tests that exercise the middleware
// without a live Casdoor / JWT pipeline.
//
// The returned restore function reverts to the previous resolver — call
// it from a defer block to avoid test pollution.
func SetUserResolver(resolver func(ctx *gin.Context) (string, []string, error)) (restore func()) {
	previous := userResolver
	userResolver = resolver
	return func() {
		userResolver = previous
	}
}

// RegisterBuiltinEnforcers registers the default access rule handlers.
// Called at startup. A simple project can skip specific handlers or
// override them with custom implementations.
func RegisterBuiltinEnforcers(entityLoader EntityLoader, memberChecker MembershipChecker) {
	RegisterAccessEnforcer(Public, func(ctx *gin.Context, rule AccessRule, userID string, roles []string) bool {
		return true
	})

	RegisterAccessEnforcer(SelfScoped, func(ctx *gin.Context, rule AccessRule, userID string, roles []string) bool {
		return true // Documentation-only; handlers filter by userId themselves
	})

	RegisterAccessEnforcer(AdminOnly, func(ctx *gin.Context, rule AccessRule, userID string, roles []string) bool {
		if IsAdmin(roles) {
			return true
		}
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":  "Access denied",
			"detail": "Administrator role required",
		})
		return false
	})

	RegisterAccessEnforcer(EntityOwner, func(ctx *gin.Context, rule AccessRule, userID string, roles []string) bool {
		if IsAdmin(roles) {
			return true
		}
		entityID := ctx.Param("id")
		ownerValue, err := entityLoader.GetOwnerField(rule.Entity, entityID, rule.Field)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "Access denied",
				"detail": "Failed to verify entity ownership",
			})
			return false
		}
		if ownerValue == userID {
			return true
		}
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":  "Access denied",
			"detail": "You do not own this entity",
		})
		return false
	})

	RegisterAccessEnforcer(GroupRole, func(ctx *gin.Context, rule AccessRule, userID string, roles []string) bool {
		if IsAdmin(roles) {
			return true
		}
		groupID := ctx.Param(rule.Param)
		allowed, err := memberChecker.CheckGroupRole(groupID, userID, rule.MinRole)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "Access denied",
				"detail": "Failed to verify group membership",
			})
			return false
		}
		if allowed {
			return true
		}
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":  "Access denied",
			"detail": "Insufficient group role",
		})
		return false
	})

	RegisterAccessEnforcer(OrgRole, func(ctx *gin.Context, rule AccessRule, userID string, roles []string) bool {
		if IsAdmin(roles) {
			return true
		}
		orgID := ctx.Param(rule.Param)
		allowed, err := memberChecker.CheckOrgRole(orgID, userID, rule.MinRole)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "Access denied",
				"detail": "Failed to verify organization membership",
			})
			return false
		}
		if allowed {
			return true
		}
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":  "Access denied",
			"detail": "Insufficient organization role",
		})
		return false
	})
}

// Layer2Enforcement returns a Gin middleware that enforces Layer 2
// business-logic authorization based on the RouteRegistry.
// Routes not registered in the registry pass through for backwards compatibility.
//
// Layer2Enforcement is self-sufficient with respect to authentication: when
// the request context already carries a `userId` (e.g. because
// AuthManagement ran earlier in the chain), it is reused as-is. Otherwise
// the middleware parses the JWT from the request itself so the declared
// RouteRegistry rule is evaluated at request time regardless of whether
// AuthManagement happens to run before or after Layer2Enforcement.
//
// Layer2Enforcement is NOT an authentication checkpoint. If the JWT is
// missing or invalid, it falls through to the next middleware (which is
// AuthManagement in production) instead of rejecting the request. This
// preserves the single source of truth for 401 responses.
func Layer2Enforcement() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		method := ctx.Request.Method
		path := ctx.FullPath()

		perm, found := RouteRegistry.Lookup(method, path)
		if !found {
			ctx.Next()
			return
		}

		userIdStr, roles, ok := resolveUserForEnforcement(ctx)
		if !ok {
			// JWT missing / invalid — Layer 2 is not an authentication
			// checkpoint. Pass through so the downstream AuthManagement
			// middleware can reject with 401.
			ctx.Next()
			return
		}

		handler := getAccessEnforcer(perm.Access.Type)
		if handler == nil {
			// Unknown access type with no registered handler — pass through
			ctx.Next()
			return
		}

		if handler(ctx, perm.Access, userIdStr, roles) {
			ctx.Next()
		}
	}
}

// resolveUserForEnforcement returns the authenticated user's ID and roles
// for the current request. If AuthManagement has already populated the
// context, the pre-resolved values are reused. Otherwise the JWT is
// parsed directly from the request.
//
// The boolean return is false when no authenticated user could be
// resolved — the caller must treat this as "pass-through", NOT as a
// rejection.
func resolveUserForEnforcement(ctx *gin.Context) (userID string, roles []string, ok bool) {
	if existing, exists := ctx.Get("userId"); exists {
		uid, _ := existing.(string)
		if uid == "" {
			return "", nil, false
		}
		rolesVal, _ := ctx.Get("userRoles")
		existingRoles, _ := rolesVal.([]string)
		return uid, existingRoles, true
	}

	uid, resolvedRoles, err := userResolver(ctx)
	if err != nil || uid == "" {
		return "", nil, false
	}
	return uid, resolvedRoles, true
}
