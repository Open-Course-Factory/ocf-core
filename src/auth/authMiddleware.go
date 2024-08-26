package authController

import (
	"fmt"
	"log"
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"

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
		userId, err := getUserIdFromToken(ctx)

		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"msg": err.Error()})
		}

		errLoadingPolicy := casdoor.Enforcer.LoadPolicy()
		if errLoadingPolicy != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error loading authorization policy"})
			return
		}
		// Casbin enforces policy, subject = user currently logged in, obj = ressource URI obtained from request path, action (http verb))
		ok, errEnforce := casdoor.Enforcer.Enforce(fmt.Sprint(userId), ctx.FullPath(), ctx.Request.Method)

		if errEnforce != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error occurred when authorizing user"})
			return
		}

		if !ok {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "You are not authorized"})
			return
		}

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

		ctx.Set("userRoles", userRoles)
		ctx.Set("userId", userId)

	}
}

func getUserIdFromToken(ctx *gin.Context) (string, error) {
	token := ctx.Request.Header.Get("Authorization")

	// WebSocket Hack
	if token == "" {
		token = ctx.Query("Authorization")
	}

	claims, err := casdoorsdk.ParseJwtToken(token)

	if err != nil {
		return "", err
	}

	userId := claims.Id
	return userId, nil
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
