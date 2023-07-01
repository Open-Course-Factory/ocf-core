package courseController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func CoursesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	courseController := NewCourseController(db)

	middleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	routes := router.Group("/courses")

	routes.POST("/generate", middleware.CheckIsLogged(), courseController.GenerateCourse)
	routes.POST("/git", middleware.CheckIsLogged(), courseController.CreateCourseFromGit)

	routes.DELETE("/:id", middleware.CheckIsLogged(), courseController.DeleteCourse)
}
