package authController

import (
	"fmt"
	"log"
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

		// Debug logging
		log.Printf("[DEBUG] User %s has roles: %v", userId, userRoles)
		log.Printf("[DEBUG] Checking access to %s %s", ctx.Request.Method, ctx.Request.URL.Path)

		// Check authorization for each role - if any role has permission, allow access
		authorized := false
		for _, role := range userRoles {
			log.Printf("[DEBUG] Checking role '%s' for access to %s %s", role, ctx.Request.Method, ctx.Request.URL.Path)
			ok, errEnforce := am.permissionService.HasPermission(role, ctx.Request.URL.Path, ctx.Request.Method)
			if errEnforce != nil {
				log.Printf("[DEBUG] Enforce error for role '%s': %v", role, errEnforce)
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error occurred when authorizing user"})
				return
			}
			log.Printf("[DEBUG] Role '%s' enforcement result: %v", role, ok)
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
			log.Printf("[DEBUG] ❌ AUTHORIZATION FAILED for user %s with roles %v trying to access %s %s", userId, userRoles, ctx.Request.Method, ctx.Request.URL.Path)

			// 🔍 AUDIT LOG: Authorization denied
			userUUID, _ := uuid.Parse(userId)
			am.auditService.LogSecurityEvent(ctx, models.AuditEventAccessDenied, &userUUID, nil,
				fmt.Sprintf("Access denied to %s %s", ctx.Request.Method, ctx.Request.URL.Path),
				models.AuditSeverityWarning)

			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "You are not authorized"})
			return
		}

		log.Printf("[DEBUG] ✅ AUTHORIZATION SUCCESS for user %s with roles %v accessing %s %s", userId, userRoles, ctx.Request.Method, ctx.Request.URL.Path)

		ctx.Set("userRoles", userRoles)
		ctx.Set("userId", userId)

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
		log.Default().Fatal("Permission Middleware has been called on a method without entity ID")
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
