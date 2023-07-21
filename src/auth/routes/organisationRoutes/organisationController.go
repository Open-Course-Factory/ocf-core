package organisationController

import (
	controller "soli/formations/src/auth/routes"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OrganisationController interface {
	GetOrganisation(ctx *gin.Context)
	GetOrganisations(ctx *gin.Context)
	AddOrganisation(ctx *gin.Context)
	EditOrganisation(ctx *gin.Context)
	DeleteOrganisation(ctx *gin.Context)
}

type organisationController struct {
	controller.GenericController
	service services.OrganisationService
	config  *config.Configuration
}

func NewOrganisationController(db *gorm.DB, config *config.Configuration) OrganisationController {

	controller := &organisationController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewOrganisationService(db),
		config:            config,
	}
	return controller
}
