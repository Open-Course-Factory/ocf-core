package controller

import (
	"log"
	"net/http"
	"reflect"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/types"
	"strings"

	pluralize "github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var methodPermissionMap = map[string]types.Permission{
	http.MethodGet:    types.PermissionTypeRead,
	http.MethodPost:   types.PermissionTypeWrite,
	http.MethodPut:    types.PermissionTypeWrite,
	http.MethodPatch:  types.PermissionTypeWrite,
	http.MethodDelete: types.PermissionTypeDelete,
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

func GetRolesFromContext(ctx *gin.Context) (*[]models.UserRole, bool, bool) {
	rawRoles, ok := ctx.Get("roles")

	if !ok {
		ctx.JSON(http.StatusOK, "[]")
		ctx.Abort()
		return nil, false, false
	}

	userRoleObjectAssociationModels, isRole := rawRoles.(*[]models.UserRole)
	return userRoleObjectAssociationModels, isRole, true
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

func HasLoggedInUserPermissionForEntity(userRoleObjectAssociations *[]models.UserRole, method string, entityName string, entityUUID uuid.UUID) bool {
	var proceed bool

	for _, userRoleObjectAssociation := range *userRoleObjectAssociations {
		if userRoleObjectAssociation.SubType == entityName {
			if reflect.DeepEqual(userRoleObjectAssociation.SubObjectID, entityUUID) {
				if types.ContainsPermissionType(userRoleObjectAssociation.Role.Permissions, types.PermissionTypeAll) ||
					types.ContainsPermissionType(userRoleObjectAssociation.Role.Permissions, methodPermissionMap[method]) {
					proceed = true
					break
				}
			}
		}

	}
	return proceed
}
