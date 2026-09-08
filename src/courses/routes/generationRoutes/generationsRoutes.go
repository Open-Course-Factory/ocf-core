package generationController

import (
	"github.com/gin-gonic/gin"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func GenerationsRoutes(router *gin.RouterGroup, db *gorm.DB) {
	generationController := NewGenerationController(db)

	routes := router.Group("/generations")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), generationController.GetGenerations)
	routes.POST("", middleware.AuthManagement(), generationController.AddGeneration)

	routes.DELETE("/:id", middleware.AuthManagement(), generationController.DeleteGeneration)
}
