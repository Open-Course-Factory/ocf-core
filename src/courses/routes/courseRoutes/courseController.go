package courseController

import (
	authServices "soli/formations/src/auth/services"
	courseServices "soli/formations/src/courses/services"

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
	authServices.GenericService
	service     courseServices.CourseService
	userService authServices.UserService
}

func NewCourseController(db *gorm.DB) CourseController {
	return &courseController{
		GenericService: authServices.NewGenericService(db),
		service:        courseServices.NewCourseService(db),
		userService:    authServices.NewUserService(db),
	}
}
