package casbin

import (
	"net/http"

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

// NewLayer2Enforcement returns a Gin middleware that enforces Layer 2
// business-logic authorization based on the RouteRegistry.
// Routes not registered in the registry pass through for backwards compatibility.
func NewLayer2Enforcement(entityLoader EntityLoader, memberChecker MembershipChecker) gin.HandlerFunc {
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

		switch perm.Access.Type {
		case Public, SelfScoped:
			ctx.Next()
			return

		case AdminOnly:
			if IsAdmin(roles) {
				ctx.Next()
				return
			}
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "Access denied",
				"detail": "Administrator role required",
			})
			return

		case EntityOwner:
			if IsAdmin(roles) {
				ctx.Next()
				return
			}
			entityID := ctx.Param("id")
			ownerValue, err := entityLoader.GetOwnerField(perm.Access.Entity, entityID, perm.Access.Field)
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":  "Access denied",
					"detail": "Failed to verify entity ownership",
				})
				return
			}
			if ownerValue == userIdStr {
				ctx.Next()
				return
			}
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "Access denied",
				"detail": "You do not own this entity",
			})
			return

		case GroupRole:
			if IsAdmin(roles) {
				ctx.Next()
				return
			}
			groupID := ctx.Param(perm.Access.Param)
			allowed, err := memberChecker.CheckGroupRole(groupID, userIdStr, perm.Access.MinRole)
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":  "Access denied",
					"detail": "Failed to verify group membership",
				})
				return
			}
			if allowed {
				ctx.Next()
				return
			}
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "Access denied",
				"detail": "Insufficient group role",
			})
			return

		case OrgRole:
			if IsAdmin(roles) {
				ctx.Next()
				return
			}
			orgID := ctx.Param(perm.Access.Param)
			allowed, err := memberChecker.CheckOrgRole(orgID, userIdStr, perm.Access.MinRole)
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":  "Access denied",
					"detail": "Failed to verify organization membership",
				})
				return
			}
			if allowed {
				ctx.Next()
				return
			}
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "Access denied",
				"detail": "Insufficient organization role",
			})
			return

		default:
			// Unknown access type — pass through for safety
			ctx.Next()
			return
		}
	}
}
