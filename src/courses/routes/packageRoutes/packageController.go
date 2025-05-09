package packageController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PackageController interface {
	AddPackage(ctx *gin.Context)
	DeletePackage(ctx *gin.Context)
	GetPackages(ctx *gin.Context)
}

type packageController struct {
	controller.GenericController
	service services.GenericService
}

func NewPackageController(db *gorm.DB) PackageController {
	return &packageController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
