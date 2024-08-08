package courseController

import (
	services "soli/formations/src/courses/services"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CourseController interface {
	GenerateCourse(ctx *gin.Context)
	AddCourse(ctx *gin.Context)
	DeleteCourse(ctx *gin.Context)
	GetCourses(ctx *gin.Context)
	CreateCourseFromGit(ctx *gin.Context)
}

type courseController struct {
	controller.GenericController
	service services.CourseService
}

func NewCourseController(db *gorm.DB) CourseController {
	return &courseController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewCourseService(db),
	}
}
