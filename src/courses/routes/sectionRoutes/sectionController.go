package sectionController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SectionController interface {
	AddSection(ctx *gin.Context)
	DeleteSection(ctx *gin.Context)
	GetSections(ctx *gin.Context)
	GetSection(ctx *gin.Context)
	EditSection(ctx *gin.Context)
}

type sectionController struct {
	controller.GenericController
	service services.GenericService
}

func NewSectionController(db *gorm.DB) SectionController {
	return &sectionController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
