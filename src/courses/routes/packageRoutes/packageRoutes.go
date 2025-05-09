package packageController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func PackagesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	packageController := NewPackageController(db)

	routes := router.Group("/packages")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), packageController.GetPackages)
	routes.POST("", middleware.AuthManagement(), packageController.AddPackage)

	routes.DELETE("/:id", middleware.AuthManagement(), packageController.DeletePackage)
}
