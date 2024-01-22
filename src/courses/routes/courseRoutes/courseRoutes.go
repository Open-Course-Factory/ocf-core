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

	middleware := &auth.AuthMiddleware{}

	routes.GET("/", middleware.AuthManagement(), courseController.GetCourses)

	routes.POST("/generate", middleware.AuthManagement(), courseController.GenerateCourse)
	routes.POST("/git", middleware.AuthManagement(), courseController.CreateCourseFromGit)

	routes.DELETE("/:id", middleware.AuthManagement(), courseController.DeleteCourse)
}
