package controller

import (
	authInterfaces "soli/formations/src/auth/interfaces"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/services"
	"soli/formations/src/entityManagement/utils"
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
	EditEntity(ctx *gin.Context)
	GetGenericService() *services.GenericService
}

type genericController struct {
	genericService            services.GenericService
	entityRegistrationService *ems.EntityRegistrationService
	enforcer                  authInterfaces.EnforcerInterface
}

// NewGenericController creates a new generic controller with the given database and enforcer.
// The enforcer parameter can be nil for testing purposes.
func NewGenericController(db *gorm.DB, enforcer authInterfaces.EnforcerInterface) GenericController {
	controller := &genericController{
		genericService:            services.NewGenericService(db, enforcer),
		entityRegistrationService: ems.GlobalEntityRegistrationService,
		enforcer:                  enforcer,
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
	return utils.KebabToPascal(singular)
}

func GetResourceNameFromPath(path string) string {

	segment := prepareEntityName(path)

	client := pluralize.NewClient()
	singular := client.Plural(segment)
	res := strings.ToLower(singular)
	return res
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
