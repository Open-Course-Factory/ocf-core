package authController

import (
	"fmt"
	"net/http"
	"soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	authModels "soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthMiddleware interface {
	AuthManagement() gin.HandlerFunc
	// IdentifyIfPresent attaches an identity when one is supplied, and never
	// rejects. For public routes whose response depends on who is asking.
	IdentifyIfPresent() gin.HandlerFunc
}

type authMiddleware struct {
	permissionService PermissionService
	auditService      auditServices.AuditService
}

func NewAuthMiddleware(db *gorm.DB) AuthMiddleware {
	return &authMiddleware{
		permissionService: NewPermissionService(),
		auditService:      auditServices.NewAuditService(db),
	}
}

// Package-level impersonation handler. Configured once at startup via
// SetImpersonationHandler. nil = impersonation disabled (tests, etc.).
var impersonationHandler gin.HandlerFunc

// SetImpersonationHandler installs the impersonation middleware that
// AuthManagement invokes after authenticating the caller. Call this once at
// app startup, after the impersonation service has been built.
//
// If h is nil, AuthManagement skips the impersonation step (this is the
// default; tests and apps without impersonation work unchanged).
//
// Why a package-level setter instead of a constructor parameter: AuthManagement
// is wired per-route from ~18 call sites via NewAuthMiddleware(db).AuthManagement().
// Threading an impersonation dependency through every caller would be invasive
// and error-prone. The setter pattern keeps the change surgical and lets tests
// install/uninstall the handler around their assertions.
func SetImpersonationHandler(h gin.HandlerFunc) {
	impersonationHandler = h
}

func (am *authMiddleware) AuthManagement() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId, tokenJTI, err := casdoor.ParseUserIDFromRequest(ctx)

		if err != nil {
			// 🔍 AUDIT LOG: Failed authentication attempt
			am.auditService.LogAuthentication(ctx, models.AuditEventLoginFailed, nil, "", "failed", err.Error())
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": err.Error()})
			return
		}

		// Check if token is blacklisted
		if isTokenBlacklisted(tokenJTI) {
			// 🔍 AUDIT LOG: Attempted use of revoked token
			userUUID, _ := uuid.Parse(userId)
			am.auditService.LogSecurityEvent(ctx, models.AuditEventAccessDenied, &userUUID, nil, "Attempted use of revoked token", models.AuditSeverityWarning)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": "token has been invalidated"})
			return
		}

		errLoadingPolicy := casdoor.Enforcer.LoadPolicy()
		if errLoadingPolicy != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error loading authorization policy"})
			return
		}

		// Get user roles first
		var userRoles []string
		userRoles, errRoles := casdoor.Enforcer.GetRolesForUser(userId)
		if errRoles != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: errRoles.Error(),
			})
			ctx.Abort()
			return
		}

		// Check authorization for each role - if any role has permission, allow access
		authorized := false
		for _, role := range userRoles {
			ok, errEnforce := am.permissionService.HasPermission(role, ctx.Request.URL.Path, ctx.Request.Method)
			if errEnforce != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error occurred when authorizing user"})
				return
			}
			if ok {
				authorized = true
				break
			}
		}

		// Also check direct user permissions (fallback for specific user permissions)
		if !authorized {
			ok, errEnforce := am.permissionService.HasPermission(fmt.Sprint(userId), ctx.Request.URL.Path, ctx.Request.Method)
			if errEnforce != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error occurred when authorizing user"})
				return
			}
			authorized = ok
		}

		if !authorized {
			// 🔍 AUDIT LOG: Authorization denied
			userUUID, _ := uuid.Parse(userId)
			am.auditService.LogSecurityEvent(ctx, models.AuditEventAccessDenied, &userUUID, nil,
				fmt.Sprintf("Access denied to %s %s", ctx.Request.Method, ctx.Request.URL.Path),
				models.AuditSeverityWarning)

			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "You are not authorized"})
			return
		}

		ctx.Set("userRoles", userRoles)
		ctx.Set("userId", userId)

		// Apply impersonation swap if configured. The handler reads
		// X-Impersonate-User and (when set + caller is admin + valid session)
		// swaps ctx.userId / ctx.userRoles to the target user. No header → no-op.
		if impersonationHandler != nil {
			impersonationHandler(ctx)
		}
	}
}

// IdentifyIfPresent authenticates the caller when a usable token is supplied and
// otherwise lets the request through anonymously. It never rejects.
//
// It exists for routes that are deliberately PUBLIC but whose response still
// depends on who is asking. The subscription-plan catalogue is the case that
// forced it: its read routes are declared Security=false so the public pricing
// page can fetch them without a session, which meant the route was mounted with
// no auth middleware at all — so userRoles was never populated and the
// VisibilityScope predicate could not recognise an administrator. Admins were
// therefore shown only catalogue plans and could not see, let alone edit, the
// hidden ones (#444).
//
// Deliberately NOT an authorization step: it performs no Casbin permission check
// and grants nothing. It only answers "is there a valid identity attached to this
// request?", so downstream read scoping can widen for admins. Any route that must
// REFUSE anonymous callers keeps using AuthManagement.
//
// Fails open by design — a malformed, expired or revoked token yields an
// anonymous request rather than an error, because the route is public. The
// consequence is only that the caller sees the public projection.
func (am *authMiddleware) IdentifyIfPresent() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId, tokenJTI, err := casdoor.ParseUserIDFromRequest(ctx)
		if err != nil || userId == "" {
			ctx.Next()
			return
		}
		if isTokenBlacklisted(tokenJTI) {
			ctx.Next()
			return
		}
		if err := casdoor.Enforcer.LoadPolicy(); err != nil {
			ctx.Next()
			return
		}
		userRoles, errRoles := casdoor.Enforcer.GetRolesForUser(userId)
		if errRoles != nil {
			ctx.Next()
			return
		}

		ctx.Set("userRoles", userRoles)
		ctx.Set("userId", userId)
		ctx.Next()
	}
}

// isTokenBlacklisted checks if a token JTI is in the blacklist
func isTokenBlacklisted(tokenJTI string) bool {
	if tokenJTI == "" {
		return false
	}

	var count int64
	sqldb.DB.Model(&authModels.TokenBlacklist{}).
		Where("token_jti = ? AND expires_at > ?", tokenJTI, time.Now()).
		Count(&count)

	return count > 0
}

func GetEntityIdFromContext(ctx *gin.Context) (uuid.UUID, bool) {
	entityID := ctx.Param("id")

	if entityID == "" {
		ctx.JSON(http.StatusBadRequest, "Entities Not Found")
		ctx.Abort()
		return uuid.Nil, false
	}

	entityUUID, errUUID := uuid.Parse(entityID)

	if errUUID != nil {
		ctx.JSON(http.StatusNotFound, "Entity Not Found")
		ctx.Abort()
		return uuid.Nil, false
	}
	return entityUUID, true
}
