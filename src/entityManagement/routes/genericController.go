package controller

import (
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
	DeleteEntity(ctx *gin.Context, scoped bool)
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

func (genericController genericController) GetGenericService() *services.GenericService {
	return &genericController.genericService
}

func GetEntityNameFromPath(path string) string {

	segment := prepareEntityName(path)

	client := pluralize.NewClient()
	singular := client.Singular(segment)
	return strings.ToUpper(string(singular[0])) + singular[1:]
}

func GetResourceNameFromPath(path string) string {

	segment := prepareEntityName(path)

	client := pluralize.NewClient()
	singular := client.Plural(segment)
	return strings.ToLower(singular)
}

func prepareEntityName(path string) string {
	path = strings.TrimRight(path, "/")

	segments := strings.Split(path, "/")
	segment := ""

	if len(segments) > 3 {
		segment = segments[3]
	} else {
		segment = segments[1]
	}
	return segment
}
