package generationController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GenerationController interface {
	AddGeneration(ctx *gin.Context)
	DeleteGeneration(ctx *gin.Context)
	GetGenerations(ctx *gin.Context)
}

type generationController struct {
	controller.GenericController
	service services.GenericService
}

func NewGenerationController(db *gorm.DB) GenerationController {
	return &generationController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
