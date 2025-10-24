package authController

import (
	"fmt"
	"log"
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	"strings"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthMiddleware interface {
	AuthManagement() gin.HandlerFunc
}

type authMiddleware struct {
}

func NewAuthMiddleware(db *gorm.DB) AuthMiddleware {
	return &authMiddleware{}
}

func (am *authMiddleware) AuthManagement() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId, tokenJTI, err := getUserIdFromToken(ctx)

		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": err.Error()})
			return
		}

		// Check if token is blacklisted
		if isTokenBlacklisted(tokenJTI) {
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
		log.Printf("[DEBUG] Checking access to %s %s", ctx.Request.Method, ctx.FullPath())

		// Check authorization for each role - if any role has permission, allow access
		authorized := false
		for _, role := range userRoles {
			log.Printf("[DEBUG] Checking role '%s' for access to %s %s", role, ctx.Request.Method, ctx.FullPath())
			ok, errEnforce := casdoor.Enforcer.Enforce(role, ctx.FullPath(), ctx.Request.Method)
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
			ok, errEnforce := casdoor.Enforcer.Enforce(fmt.Sprint(userId), ctx.FullPath(), ctx.Request.Method)
			if errEnforce != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error occurred when authorizing user"})
				return
			}
			authorized = ok
		}

		if !authorized {
			log.Printf("[DEBUG] ❌ AUTHORIZATION FAILED for user %s with roles %v trying to access %s %s", userId, userRoles, ctx.Request.Method, ctx.FullPath())
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "You are not authorized"})
			return
		}

		log.Printf("[DEBUG] ✅ AUTHORIZATION SUCCESS for user %s with roles %v accessing %s %s", userId, userRoles, ctx.Request.Method, ctx.FullPath())

		ctx.Set("userRoles", userRoles)
		ctx.Set("userId", userId)

	}
}

func getUserIdFromToken(ctx *gin.Context) (string, string, error) {
	token := ctx.Request.Header.Get("Authorization")

	// WebSocket Hack
	if token == "" {
		token = ctx.Query("Authorization")
	}

	// Enlever le préfixe "Bearer " s'il est présent
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	} else if strings.HasPrefix(token, "bearer ") {
		token = strings.TrimPrefix(token, "bearer ")
	}

	// Vérifier que le token n'est pas vide après nettoyage
	if token == "" {
		return "", "", fmt.Errorf("missing or invalid authorization token")
	}

	claims, err := casdoorsdk.ParseJwtToken(token)
	if err != nil {
		return "", "", err
	}

	userId := claims.Id
	tokenJTI := claims.ID // JWT ID claim
	return userId, tokenJTI, nil
}

// isTokenBlacklisted checks if a token JTI is in the blacklist
func isTokenBlacklisted(tokenJTI string) bool {
	if tokenJTI == "" {
		return false
	}

	var count int64
	sqldb.DB.Model(&models.TokenBlacklist{}).
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
