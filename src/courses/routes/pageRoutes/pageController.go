package pageController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PageController interface {
	AddPage(ctx *gin.Context)
	DeletePage(ctx *gin.Context)
	GetPages(ctx *gin.Context)
	GetPage(ctx *gin.Context)
	EditPage(ctx *gin.Context)
}

type pageController struct {
	controller.GenericController
	service services.GenericService
}

func NewPageController(db *gorm.DB) PageController {
	return &pageController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
