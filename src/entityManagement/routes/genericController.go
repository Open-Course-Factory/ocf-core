package controller

import (
	"reflect"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/services"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GenericController interface {
	AddEntity(ctx *gin.Context)
	GetEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetGenericService() *services.GenericService
}

type genericController struct {
	genericService            services.GenericService
	entityRegistrationService *ems.EntityRegistrationService
}

func NewGenericController(db *gorm.DB) GenericController {
	controller := &genericController{
		genericService:            services.NewGenericService(db),
		entityRegistrationService: ems.GlobalEntityRegistrationService,
	}

	return controller
}

// used in get
func (genericController genericController) appendEntityFromResult(entityName string, item interface{}, entitiesDto []interface{}) ([]interface{}, bool) {
	result, ko := genericController.getEntityFromResult(entityName, item)
	if !ko {
		entitiesDto = append(entitiesDto, result)
		return entitiesDto, false
	}

	return nil, true
}

// used in post and get
func (genericController genericController) getEntityFromResult(entityName string, item interface{}) (interface{}, bool) {
	var result interface{}
	if funcRef, ok := genericController.entityRegistrationService.GetConversionFunction(entityName, ems.OutputModelToDto); ok {
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

func GetEntityNameFromPath(path string) string {

	path = strings.TrimRight(path, "/")

	segments := strings.Split(path, "/")
	segment := ""

	if len(segments) > 3 {
		segment = segments[3]
	} else {
		segment = segments[1]
	}

	client := pluralize.NewClient()
	singular := client.Singular(segment)
	return strings.ToUpper(string(singular[0])) + singular[1:]
}
