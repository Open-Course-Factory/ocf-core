package controller

import (
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var funcMap = make(map[string]interface{})

type GenericController interface {
	GetEntity(ctx *gin.Context)
	GetEntitiesWithPermissionCheck(ctx *gin.Context)
	GetEntitiesWithoutPermissionCheck(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetGenericService() *services.GenericService
}

type genericController struct {
	genericService services.GenericService
}

func NewGenericController(db *gorm.DB) GenericController {
	funcMap["RoleModelToRoleOutput"] = dto.RoleModelToRoleOutput
	funcMap["UserModelToUserOutput"] = dto.UserModelToUserOutput
	funcMap["GroupModelToGroupOutput"] = dto.GroupModelToGroupOutput
	funcMap["OrganisationModelToOrganisationOutput"] = dto.OrganisationModelToOrganisationOutput

	controller := &genericController{
		genericService: services.NewGenericService(db),
	}

	return controller
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

func (genericController genericController) GetGenericService() *services.GenericService {
	return &genericController.genericService
}
