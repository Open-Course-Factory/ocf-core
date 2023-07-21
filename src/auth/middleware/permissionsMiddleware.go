package middleware

import (
	"net/http"
	controller "soli/formations/src/auth/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PermissionsMiddleware struct {
	DB *gorm.DB
}

func (opm PermissionsMiddleware) IsAuthorized() gin.HandlerFunc {

	return func(ctx *gin.Context) {

		permissionsArray, isPermissionsArray, shouldReturn := controller.GetPermissionsFromContext(ctx)
		if shouldReturn {
			return
		}

		if isPermissionsArray {
			entityUUID, shouldReturn := controller.GetEntityIdFromContext(ctx)
			if shouldReturn {
				return
			}

			entityName := controller.GetEntityNameFromPath(ctx.FullPath())

			proceed := controller.HasLoggedInUserPermissionForEntity(permissionsArray, ctx.Request.Method, entityName, entityUUID)

			if !proceed {
				ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
				ctx.Abort()
				return
			} else {
				ctx.Next()
			}

		} else {
			ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
			ctx.Abort()
			return
		}

	}

}
