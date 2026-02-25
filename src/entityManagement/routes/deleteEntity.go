package controller

import (
	stderrors "errors"
	"net/http"

	"soli/formations/src/auth/errors"
	entityErrors "soli/formations/src/entityManagement/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (genericController genericController) DeleteEntity(ctx *gin.Context, scoped bool) {

	id, parseErr := uuid.Parse(ctx.Param("id"))
	if errors.HandleError(http.StatusBadRequest, parseErr, ctx) {
		return
	}

	entityName := GetEntityNameFromPath(ctx.FullPath())
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	entity, getEntityError := genericController.genericService.GetEntity(id, entityModelInterface, entityName, nil)
	if errors.HandleError(http.StatusNotFound, getEntityError, ctx) {
		return
	}

	errorDelete := genericController.genericService.DeleteEntity(id, entity, scoped)
	if errorDelete != nil {
		statusCode := http.StatusInternalServerError // default
		var entityErr *entityErrors.EntityError
		if stderrors.As(errorDelete, &entityErr) {
			statusCode = entityErr.HTTPStatus
		}
		if errors.HandleError(statusCode, errorDelete, ctx) {
			return
		}
	}

	// Skip enforcer cleanup if not initialized (e.g., in tests)
	if genericController.enforcer != nil {
		errPolicyLoading := genericController.enforcer.LoadPolicy()
		if errors.HandleError(http.StatusInternalServerError, errPolicyLoading, ctx) {
			return
		}

		resourceName := GetResourceNameFromPath(ctx.FullPath())

		_, errRemovingPolicy := genericController.enforcer.RemoveFilteredPolicy(1, "/api/v1/"+resourceName+"/"+id.String())
		if errors.HandleError(http.StatusInternalServerError, errRemovingPolicy, ctx) {
			return
		}
	}

	ctx.JSON(http.StatusNoContent, "Done")
}
