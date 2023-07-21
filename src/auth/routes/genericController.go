package controller

import (
	"fmt"
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
	"strings"

	pluralize "github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var funcMap = make(map[string]interface{})

type GenericController interface {
	GetEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
}

type genericController struct {
	genericService services.GenericService
}

func NewGenericController(db *gorm.DB) GenericController {
	funcMap["RoleModelToRoleOutput"] = dto.RoleModelToRoleOutput
	funcMap["UserModelToUserOutput"] = dto.UserModelToUserOutput
	funcMap["GroupModelToGroupOutput"] = dto.GroupModelToGroupOutput
	funcMap["OrganisationModelToOrganisationOutput"] = dto.OrganisationModelToOrganisationOutput
	funcMap["PermissionModelToPermissionOutput"] = dto.PermissionModelToPermissionOutput

	controller := &genericController{
		genericService: services.NewGenericService(db),
	}
	return controller
}

func (genericController genericController) getEntityModelInterface(entityName string) interface{} {
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

func (genericController genericController) extractSingularResource(path string) string {
	fmt.Println(path)

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

func (genericController genericController) appendEntityFromResult(funcName string, item interface{}, entitiesDto []interface{}) ([]interface{}, bool) {
	if funcRef, ok := funcMap[funcName]; ok {
		val := reflect.ValueOf(funcRef)

		if val.IsValid() && val.Kind() == reflect.Func {
			args := []reflect.Value{reflect.ValueOf(item)}
			entityDto := val.Call(args)
			if len(entityDto) == 1 {
				result := entityDto[0].Interface()

				entitiesDto = append(entitiesDto, result)
			}

		} else {
			return nil, true
		}
	} else {
		return nil, true
	}
	return entitiesDto, false
}

func (genericController genericController) getEntityFromResult(funcName string, item interface{}) (interface{}, bool) {
	var result interface{}
	if funcRef, ok := funcMap[funcName]; ok {
		val := reflect.ValueOf(funcRef)

		if val.IsValid() && val.Kind() == reflect.Func {
			args := []reflect.Value{reflect.ValueOf(item)}
			entityDto := val.Call(args)
			if len(entityDto) == 1 {
				result = entityDto[0].Interface()
			}

		} else {
			return nil, true
		}
	} else {
		return nil, true
	}
	return result, false
}
