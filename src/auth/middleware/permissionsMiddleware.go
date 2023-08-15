package middleware

import (
	"net/http"
	"soli/formations/src/auth/models"
	controller "soli/formations/src/auth/routes"
	"soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PermissionsMiddleware struct {
	DB *gorm.DB
}

func (opm PermissionsMiddleware) IsAuthorized() gin.HandlerFunc {

	return func(ctx *gin.Context) {

		permissionsArray, isPermissionsArray, permissionFound := controller.GetPermissionsFromContext(ctx)
		if !permissionFound {
			return
		}

		permissionService := services.NewPermissionService(opm.DB)
		isUserInstanceAdmin := permissionService.IsUserInstanceAdmin(permissionsArray)
		if isUserInstanceAdmin {
			ctx.Next()
		}

		if isPermissionsArray {
			switch ctx.Request.Method {
			case http.MethodPost:
				ctx.Next()
			case http.MethodGet:
				_, idFound := controller.GetEntityIdFromContext(ctx)
				if idFound {
					opm.callAboutSpecificEntityWithId(ctx, permissionsArray)
				} else {
					ctx.Next()
				}
			case http.MethodPut:
				permissionFound = opm.callAboutSpecificEntityWithId(ctx, permissionsArray)
			case http.MethodPatch:
				permissionFound = opm.callAboutSpecificEntityWithId(ctx, permissionsArray)
			case http.MethodDelete:
				permissionFound = opm.callAboutSpecificEntityWithId(ctx, permissionsArray)
			default:
				ctx.JSON(http.StatusForbidden, "Unknown HTTP Method fot this endpoint")
				ctx.Abort()
				return
			}

			if !permissionFound {
				ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
				ctx.Abort()
				return
			}

		} else {
			ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
			ctx.Abort()
			return
		}

	}

}

func (PermissionsMiddleware) callAboutSpecificEntityWithId(ctx *gin.Context, permissionsArray *[]models.Permission) bool {
	entityUUID, idFound := controller.GetEntityIdFromContext(ctx)
	if !idFound {
		return false
	}

	entityName := controller.GetEntityNameFromPath(ctx.FullPath())

	proceed := controller.HasLoggedInUserPermissionForEntity(permissionsArray, ctx.Request.Method, entityName, entityUUID)

	if !proceed {
		ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
		ctx.Abort()
		return false
	} else {
		ctx.Next()
	}
	return true
}
