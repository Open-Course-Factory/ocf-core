package middleware

import (
	"log"
	"net/http"
	"reflect"
	"soli/formations/src/auth/models"
	controller "soli/formations/src/auth/routes"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionsMiddleware struct {
	DB *gorm.DB
}

var methodPermissionMap = make(map[string]models.PermissionType)

func (opm PermissionsMiddleware) getEntityIdFromPermission(permission models.Permission, entityName string) uuid.UUID {
	switch entityName {
	case "Organisation":
		return *permission.OrganisationID
	case "Group":
		return *permission.GroupID
	case "Role":
		return *permission.RoleID
	case "User":
		return *permission.UserID
	}
	return uuid.Nil
}

func (opm PermissionsMiddleware) IsAuthorized() gin.HandlerFunc {
	methodPermissionMap[http.MethodGet] = models.PermissionTypeRead
	methodPermissionMap[http.MethodPost] = models.PermissionTypeWrite
	methodPermissionMap[http.MethodPut] = models.PermissionTypeWrite
	methodPermissionMap[http.MethodPatch] = models.PermissionTypeWrite
	methodPermissionMap[http.MethodDelete] = models.PermissionTypeDelete

	return func(ctx *gin.Context) {

		var proceed bool

		rawPermissions, ok := ctx.Get("permissions")

		if !ok {
			ctx.JSON(http.StatusOK, "[]")
			ctx.Abort()
			return
		}

		permissionModels, isPermission := rawPermissions.(*[]models.Permission)

		if isPermission {
			entityID := ctx.Param("id")

			// if we are not in the case of a request dedicated to a specific entity, we cannot check here
			if entityID == "" {
				//should not happen
				log.Default().Fatal("Permission Middleware has been called on a method without organisation ID")
				ctx.Next()
			}

			entityUUID, errUUID := uuid.Parse(entityID)

			// if the ID does not exists
			if errUUID != nil {
				ctx.JSON(http.StatusNotFound, "Entity Not Found")
				ctx.Abort()
				return
			}

			entityName := controller.GetEntityNameFromPath(ctx.FullPath())

			for _, permission := range *permissionModels {
				permissionEntityUuid := opm.getEntityIdFromPermission(permission, entityName)
				if reflect.DeepEqual(permissionEntityUuid, entityUUID) {
					if models.ContainsPermissionType(permission.PermissionTypes, models.PermissionTypeAll) ||
						models.ContainsPermissionType(permission.PermissionTypes, methodPermissionMap[ctx.Request.Method]) {
						proceed = true
						break
					}
				}
			}

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
