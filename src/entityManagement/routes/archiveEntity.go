package controller

import (
	"errors"
	"net/http"
	"time"

	authErrors "soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityErrors "soli/formations/src/entityManagement/errors"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func init() {
	ems.SetArchiveActionHandlerFactory(ArchiveActionFactory)
}

// ArchiveActionFactory builds the handler behind the synthesized
// POST /{entities}/:id/archive (archive=true) and /unarchive (archive=false)
// actions. It answers 200 with the entity's output DTO, 404 for an unknown id,
// 403 when a Before* hook refused with the permission-denied error, and 409
// when a Before* hook refused for any other reason.
func ArchiveActionFactory(entityName string, archive bool) entityManagementInterfaces.ActionHandlerFactory {
	return func(db *gorm.DB) gin.HandlerFunc {
		genericService := services.NewGenericService(db, nil)
		return func(ctx *gin.Context) {
			id, err := uuid.Parse(ctx.Param("id"))
			if authErrors.HandleError(http.StatusBadRequest, err, ctx) {
				return
			}

			var at *time.Time
			if archive {
				now := time.Now()
				at = &now
			}

			entity, err := genericService.SetArchived(entityName, id, at, ctx.GetString("userId"), ctx.GetStringSlice("userRoles"))
			if err != nil {
				authErrors.HandleError(archiveErrorStatus(err), err, ctx)
				return
			}

			dto, converted := genericService.GetEntityFromResult(entityName, entity)
			if converted {
				authErrors.HandleError(http.StatusInternalServerError, errors.New("could not convert entity to its output DTO"), ctx)
				return
			}
			ctx.JSON(http.StatusOK, dto)
		}
	}
}

// archiveErrorStatus maps a SetArchived error to its HTTP status. A hook that
// refused with an opaque error is a 409 (the row's state forbids the change);
// a structured EntityError keeps its own status, so the permission-denied
// sentinel surfaces as 403 and a missing row as 404.
func archiveErrorStatus(err error) int {
	var structured *entityErrors.EntityError
	if !errors.As(err, &structured) {
		return http.StatusInternalServerError
	}
	if structured.Code == entityErrors.ErrHookExecutionFailed.Code {
		return http.StatusConflict
	}
	return structured.HTTPStatus
}
