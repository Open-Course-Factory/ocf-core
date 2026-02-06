package courseController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func CoursesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	courseController := NewCourseController(db)

	routes := router.Group("/courses")
	generationRoutes := router.Group("/generations")

	middleware := auth.NewAuthMiddleware(db)

	routes.POST("/git", middleware.AuthManagement(), courseController.CreateCourseFromGit)
	routes.POST("/source", middleware.AuthManagement(), courseController.CreateCourseFromSource)

	// Route de génération modifiée (maintenant asynchrone)
	routes.POST("/generate", middleware.AuthManagement(), courseController.GenerateCourse)

	// Version management routes
	routes.GET("/versions", middleware.AuthManagement(), courseController.GetCourseVersions)
	routes.GET("/by-version", middleware.AuthManagement(), courseController.GetCourseByVersion)

	// Nouvelles routes pour la gestion des générations
	generationRoutes.GET("/:id/status", middleware.AuthManagement(), courseController.GetGenerationStatus)
	generationRoutes.GET("/:id/download", middleware.AuthManagement(), courseController.DownloadGenerationResults)
	generationRoutes.POST("/:id/retry", middleware.AuthManagement(), courseController.RetryGeneration)
}
