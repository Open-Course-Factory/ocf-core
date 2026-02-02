package courseController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
	authMiddleware "soli/formations/src/auth/middleware"
)

func CoursesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	courseController := NewCourseController(db)

	routes := router.Group("/courses")
	generationRoutes := router.Group("/generations")

	middleware := auth.NewAuthMiddleware(db)
	verificationMiddleware := authMiddleware.NewEmailVerificationMiddleware(db)

	// Email verification is checked AFTER auth middleware to ensure userId is in context
	routes.POST("/git", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.CreateCourseFromGit)
	routes.POST("/source", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.CreateCourseFromSource)

	// Route de génération modifiée (maintenant asynchrone)
	routes.POST("/generate", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.GenerateCourse)

	// Version management routes
	routes.GET("/versions", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.GetCourseVersions)
	routes.GET("/by-version", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.GetCourseByVersion)

	// Nouvelles routes pour la gestion des générations
	generationRoutes.GET("/:id/status", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.GetGenerationStatus)
	generationRoutes.GET("/:id/download", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.DownloadGenerationResults)
	generationRoutes.POST("/:id/retry", middleware.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), courseController.RetryGeneration)
}
