// src/courses/routes/courseRoutes/courseController.go
package courseController

import (
	"soli/formations/src/auth/casdoor"
	authInterfaces "soli/formations/src/auth/interfaces"
	"soli/formations/src/courses/models"
	services "soli/formations/src/courses/services"
	controller "soli/formations/src/entityManagement/routes"
	emServices "soli/formations/src/entityManagement/services"
	workerServices "soli/formations/src/worker/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CourseController interface {
	GenerateCourse(ctx *gin.Context)
	CreateCourseFromGit(ctx *gin.Context)
	CreateCourseFromSource(ctx *gin.Context)

	// Version management
	GetCourseVersions(ctx *gin.Context)
	GetCourseByVersion(ctx *gin.Context)

	GetGenerationStatus(ctx *gin.Context)
	DownloadGenerationResults(ctx *gin.Context)
	RetryGeneration(ctx *gin.Context)
}

type courseController struct {
	controller.GenericController
	service     services.CourseService
	generations emServices.GenericService
}

// ownedGeneration loads the generation if the caller may reach it, else a not-found error.
func (c courseController) ownedGeneration(ctx *gin.Context, id uuid.UUID) (*models.Generation, error) {
	return services.GetOwnedGeneration(c.generations, id, ctx.GetString("userId"), ctx.GetStringSlice("userRoles"))
}

func NewCourseController(db *gorm.DB) CourseController {
	return &courseController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewCourseService(db),
		generations:       emServices.NewGenericService(db, casdoor.Enforcer),
	}
}

func NewCourseControllerWithDependencies(db *gorm.DB, workerService workerServices.WorkerService, casdoorService authInterfaces.CasdoorService, packageService workerServices.GenerationPackageService, genericService emServices.GenericService) CourseController {
	return &courseController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewCourseServiceWithDependencies(db, workerService, packageService, casdoorService, genericService),
		generations:       genericService,
	}
}
