// src/courses/routes/courseRoutes/courseController.go
package courseController

import (
	"errors"
	"net/http"

	"soli/formations/src/auth/casdoor"
	authErrors "soli/formations/src/auth/errors"
	authInterfaces "soli/formations/src/auth/interfaces"
	"soli/formations/src/courses/models"
	services "soli/formations/src/courses/services"
	entityErrors "soli/formations/src/entityManagement/errors"
	controller "soli/formations/src/entityManagement/routes"
	emServices "soli/formations/src/entityManagement/services"
	"soli/formations/src/utils"
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
	db          *gorm.DB
}

// ownedGeneration loads the generation if the caller owns it and its course. Otherwise it
// answers the request itself: the same 404 for missing and not yours, a bare 500 on a database error.
func (c courseController) ownedGeneration(ctx *gin.Context, id uuid.UUID) (*models.Generation, bool) {
	userID, roles := ctx.GetString("userId"), ctx.GetStringSlice("userRoles")
	generation, err := services.GetOwnedGeneration(c.generations, id, userID, roles)
	owns := false
	if err == nil {
		owns, err = services.CallerOwnsCourse(c.db, generation.CourseID, userID, roles)
		if err == nil && !owns {
			err = entityErrors.NewEntityNotFound("Generation", id)
		}
	}
	if err == nil {
		return generation, true
	}
	var entityErr *entityErrors.EntityError
	if errors.As(err, &entityErr) && entityErr.Code == entityErrors.ErrEntityNotFound.Code {
		authErrors.Respond(ctx, http.StatusNotFound, "Génération non trouvée: "+err.Error())
	} else {
		utils.Error("generation %s lookup failed: %v", id, err)
		authErrors.Respond(ctx, http.StatusInternalServerError, "Erreur interne")
	}
	return nil, false
}

func NewCourseController(db *gorm.DB) CourseController {
	return &courseController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewCourseService(db),
		generations:       emServices.NewGenericService(db, casdoor.Enforcer),
		db:                db,
	}
}

func NewCourseControllerWithDependencies(db *gorm.DB, workerService workerServices.WorkerService, casdoorService authInterfaces.CasdoorService, packageService workerServices.GenerationPackageService, genericService emServices.GenericService) CourseController {
	return &courseController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewCourseServiceWithDependencies(db, workerService, packageService, casdoorService, genericService),
		generations:       genericService,
		db:                db,
	}
}
