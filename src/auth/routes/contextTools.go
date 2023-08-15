package controller

import (
	"log"
	"net/http"
	"reflect"
	"soli/formations/src/auth/models"
	"strings"

	pluralize "github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var methodPermissionMap = map[string]models.PermissionType{
	http.MethodGet:    models.PermissionTypeRead,
	http.MethodPost:   models.PermissionTypeWrite,
	http.MethodPut:    models.PermissionTypeWrite,
	http.MethodPatch:  models.PermissionTypeWrite,
	http.MethodDelete: models.PermissionTypeDelete,
}

func GetEntityModelInterface(entityName string) interface{} {
	var result interface{}
	switch entityName {
	case "Role":
		result = models.Role{}
	case "User":
		result = models.User{}
	case "Group":
		result = models.Group{}
	case "Organisation":
		result = models.Organisation{}
	case "Permission":
		result = models.Permission{}
	}
	return result
}

func GetEntityNameFromPath(path string) string {

	// Trim any trailing slashes
	path = strings.TrimRight(path, "/")

	// Split the path into segments
	segments := strings.Split(path, "/")

	// Take resource name segment
	segment := segments[3]

	client := pluralize.NewClient()
	singular := client.Singular(segment)
	return strings.ToUpper(string(singular[0])) + singular[1:]
}

func GetPermissionsFromContext(ctx *gin.Context) (*[]models.Permission, bool, bool) {
	rawPermissions, ok := ctx.Get("permissions")

	if !ok {
		ctx.JSON(http.StatusOK, "[]")
		ctx.Abort()
		return nil, false, false
	}

	permissionModels, isPermission := rawPermissions.(*[]models.Permission)
	return permissionModels, isPermission, true
}

func GetEntityIdFromContext(ctx *gin.Context) (uuid.UUID, bool) {
	entityID := ctx.Param("id")

	if entityID == "" {
		log.Default().Fatal("Permission Middleware has been called on a method without entity ID")
		ctx.Next()
	}

	entityUUID, errUUID := uuid.Parse(entityID)

	if errUUID != nil {
		ctx.JSON(http.StatusNotFound, "Entity Not Found")
		ctx.Abort()
		return uuid.Nil, false
	}
	return entityUUID, true
}

func HasLoggedInUserPermissionForEntity(permissionModels *[]models.Permission, method string, entityName string, entityUUID uuid.UUID) bool {
	var proceed bool

	for _, permission := range *permissionModels {
		permissionEntityUuid := GetEntityIdFromPermission(permission, entityName)
		if reflect.DeepEqual(permissionEntityUuid, entityUUID) {
			if models.ContainsPermissionType(permission.PermissionTypes, models.PermissionTypeAll) ||
				models.ContainsPermissionType(permission.PermissionTypes, methodPermissionMap[method]) {
				proceed = true
				break
			}
		}
	}
	return proceed
}

func GetEntityIdFromPermission(permission models.Permission, entityName string) uuid.UUID {
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
