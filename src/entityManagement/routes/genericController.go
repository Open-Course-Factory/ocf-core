package controller

import (
	"reflect"
	authDto "soli/formations/src/auth/dto"
	coursesDto "soli/formations/src/courses/dto"
	"soli/formations/src/entityManagement/services"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var funcMap = make(map[string]interface{})

type GenericController interface {
	GetEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetGenericService() *services.GenericService
}

type genericController struct {
	genericService services.GenericService
}

func NewGenericController(db *gorm.DB) GenericController {
	funcMap["SshkeyModelToSshkeyOutput"] = authDto.SshKeyModelToSshKeyOutput
	funcMap["SessionModelToSessionOutput"] = coursesDto.SessionModelToSessionOutput
	funcMap["CourseModelToCourseOutput"] = coursesDto.CourseModelToCourseOutput

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
