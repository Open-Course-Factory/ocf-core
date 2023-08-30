package controller

import (
	"log"
	"net/http"
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

func GetEntityNameFromPath(path string) string {

	// Trim any trailing slashes
	path = strings.TrimRight(path, "/")

	// Split the path into segments
	segments := strings.Split(path, "/")
	segment := ""

	// Take resource name segment
	if len(segments) > 3 {
		segment = segments[3]
	} else {
		segment = segments[1]
	}

	// Take resource name and resource type

	client := pluralize.NewClient()
	singular := client.Singular(segment)
	return strings.ToUpper(string(singular[0])) + singular[1:]
}

func GetRolesFromContext(ctx *gin.Context) (*[]models.UserRoles, bool, bool) {
	rawRoles, ok := ctx.Get("roles")

	if !ok {
		ctx.JSON(http.StatusOK, "[]")
		ctx.Abort()
		return nil, false, false
	}

	userRoleObjectAssociationModels, isRole := rawRoles.(*[]models.UserRoles)
	return userRoleObjectAssociationModels, isRole, true
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

func HasUserRolesPermissionForEntity(userRoleObjectAssociations *[]models.UserRoles, method string, entityName string, entityUUID uuid.UUID) bool {
	var proceed bool

	for _, userRoleObjectAssociation := range *userRoleObjectAssociations {
		if userRoleObjectAssociation.SubType == entityName {
			if userRoleObjectAssociation.SubObjectID.String() == entityUUID.String() {
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
