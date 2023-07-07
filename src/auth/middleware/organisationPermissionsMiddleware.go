package middleware

import (
	"log"
	"net/http"
	"reflect"
	"soli/formations/src/auth/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganisationPermissionsMiddleware struct {
	DB *gorm.DB
}

func (opm OrganisationPermissionsMiddleware) IsAuthorized() gin.HandlerFunc {
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
			organisationID := ctx.Param("id")

			// if we are not in the case of a request dedicated to a specific organisation, we cannot check here
			if organisationID == "" {
				//should not happen
				log.Default().Fatal("Organisation Permission Middleware has been called on a method without organisation ID")
				ctx.Next()
			}

			organisationUUID, errUUID := uuid.Parse(organisationID)

			// if the ID does not exists
			if errUUID != nil {
				ctx.JSON(http.StatusNotFound, "Organisation Not Found")
				ctx.Abort()
				return
			}

			methodPermissionMap := make(map[string]models.PermissionType)
			methodPermissionMap[http.MethodGet] = models.PermissionTypeRead
			methodPermissionMap[http.MethodPost] = models.PermissionTypeWrite
			methodPermissionMap[http.MethodPut] = models.PermissionTypeWrite
			methodPermissionMap[http.MethodPatch] = models.PermissionTypeWrite
			methodPermissionMap[http.MethodDelete] = models.PermissionTypeDelete

			for _, permission := range *permissionModels {
				if reflect.DeepEqual(permission.OrganisationID, &organisationUUID) {
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
