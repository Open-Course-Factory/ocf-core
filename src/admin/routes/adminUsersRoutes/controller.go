package adminUsersRoutes

import (
	"net/http"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
)

// ListAllCasdoorUsers is a swappable function reference so tests can
// inject a controlled list of Casdoor users without hitting the real
// Casdoor server. Production path delegates to the SDK.
var ListAllCasdoorUsers = func() ([]*casdoorsdk.User, error) {
	return casdoorsdk.GetUsers()
}

// NewListUsersHandler returns the gin handler for
// GET /admin/users-with-memberships. The handler:
//
//   1. Lists every Casdoor user via the swappable ListAllCasdoorUsers var.
//   2. Builds an isAdmin predicate from the Casbin enforcer's role bindings.
//   3. Joins each user against the organization_members and group_members
//      tables to produce a UserListing slice.
//
// The endpoint is admin-only — Layer 1 (RBAC) is enforced by the
// AuthManagement middleware, Layer 2 (AdminOnly) by the global
// Layer2Enforcement middleware via the RouteRegistry declaration in
// permissions.go.
func NewListUsersHandler(db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		users, err := ListAllCasdoorUsers()
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "user_lookup_failed"})
			return
		}

		isAdmin := func(uid string) bool {
			if casdoor.Enforcer == nil {
				return false
			}
			roles, err := casdoor.Enforcer.GetRolesForUser(uid)
			if err != nil {
				return false
			}
			for _, r := range roles {
				if strings.EqualFold(r, "administrator") || strings.EqualFold(r, "admin") {
					return true
				}
			}
			return false
		}

		listings, err := BuildUserListings(users, db, isAdmin)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "build_failed"})
			return
		}

		ctx.JSON(http.StatusOK, listings)
	}
}
