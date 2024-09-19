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

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), courseController.GetCourses)
	routes.GET("/:id", middleware.AuthManagement(), courseController.GetCourse)

	routes.POST("", middleware.AuthManagement(), courseController.AddCourse)
	routes.POST("/generate", middleware.AuthManagement(), courseController.GenerateCourse)
	routes.POST("/git", middleware.AuthManagement(), courseController.CreateCourseFromGit)
	routes.PATCH("/:id", middleware.AuthManagement(), courseController.EditCourse)

	routes.DELETE("/:id", middleware.AuthManagement(), courseController.DeleteCourse)
}
