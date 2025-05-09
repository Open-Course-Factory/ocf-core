package themeController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func ThemesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	themeController := NewThemeController(db)

	routes := router.Group("/themes")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), themeController.GetThemes)
	routes.GET("/:id", middleware.AuthManagement(), themeController.GetTheme)
	routes.POST("", middleware.AuthManagement(), themeController.AddTheme)
	routes.PATCH("/:id", middleware.AuthManagement(), themeController.EditTheme)

	routes.DELETE("/:id", middleware.AuthManagement(), themeController.DeleteTheme)
}
