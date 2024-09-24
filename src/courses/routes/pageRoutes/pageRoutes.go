package pageController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func PagesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	pageController := NewPageController(db)

	routes := router.Group("/pages")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), pageController.GetPages)
	routes.GET("/:id", middleware.AuthManagement(), pageController.GetPage)
	routes.POST("", middleware.AuthManagement(), pageController.AddPage)
	routes.PATCH("/:id", middleware.AuthManagement(), pageController.EditPage)

	routes.DELETE("/:id", middleware.AuthManagement(), pageController.DeletePage)
}
