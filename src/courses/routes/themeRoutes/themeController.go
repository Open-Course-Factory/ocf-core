package themeController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ThemeController interface {
	AddTheme(ctx *gin.Context)
	DeleteTheme(ctx *gin.Context)
	GetThemes(ctx *gin.Context)
	GetTheme(ctx *gin.Context)
	EditTheme(ctx *gin.Context)
}

type themeController struct {
	controller.GenericController
	service services.GenericService
}

func NewThemeController(db *gorm.DB) ThemeController {
	return &themeController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
