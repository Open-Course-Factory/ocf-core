// src/courses/routes/courseRoutes/courseController.go
package courseController

import (
	authInterfaces "soli/formations/src/auth/interfaces"
	services "soli/formations/src/courses/services"
	controller "soli/formations/src/entityManagement/routes"
	emServices "soli/formations/src/entityManagement/services"
	workerServices "soli/formations/src/worker/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CourseController interface {
	GenerateCourse(ctx *gin.Context)
	CreateCourseFromGit(ctx *gin.Context)

	GetGenerationStatus(ctx *gin.Context)
	DownloadGenerationResults(ctx *gin.Context)
	RetryGeneration(ctx *gin.Context)
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

func NewCourseControllerWithDependencies(db *gorm.DB, workerService workerServices.WorkerService, casdoorService authInterfaces.CasdoorService, packageService workerServices.GenerationPackageService, genericService emServices.GenericService) CourseController {
	return &courseController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewCourseServiceWithDependencies(db, workerService, packageService, casdoorService, genericService),
	}
}
