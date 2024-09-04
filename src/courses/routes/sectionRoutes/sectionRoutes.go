package sectionController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func SectionsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	sectionController := NewSectionController(db)

	routes := router.Group("/sections")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), sectionController.GetSections)
	routes.GET("/:id", middleware.AuthManagement(), sectionController.GetSection)
	routes.POST("", middleware.AuthManagement(), sectionController.AddSection)
	routes.PATCH("/:id", middleware.AuthManagement(), sectionController.EditSection)

	routes.DELETE("/:id", middleware.AuthManagement(), sectionController.DeleteSection)
}
