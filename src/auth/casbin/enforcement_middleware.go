package casbin

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

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
func Layer2Enforcement() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		method := ctx.Request.Method
		path := ctx.FullPath()

		perm, found := RouteRegistry.Lookup(method, path)
		if !found {
			ctx.Next()
			return
		}

		userId, _ := ctx.Get("userId")
		userIdStr, _ := userId.(string)

		rolesVal, _ := ctx.Get("userRoles")
		roles, _ := rolesVal.([]string)

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
