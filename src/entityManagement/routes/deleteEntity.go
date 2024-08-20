package controller

import (
	"net/http"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (genericController genericController) DeleteEntity(ctx *gin.Context) {

	id, parseErr := uuid.Parse(ctx.Param("id"))
	if errors.HandleError(http.StatusBadRequest, parseErr, ctx) {
		return
	}

	entityName := GetEntityNameFromPath(ctx.FullPath())
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	entity, getEntityError := genericController.genericService.GetEntity(id, entityModelInterface)
	if errors.HandleError(http.StatusNotFound, getEntityError, ctx) {
		return
	}

	errorDelete := genericController.genericService.DeleteEntity(id, entity)
	if errors.HandleError(http.StatusNotFound, errorDelete, ctx) {
		return
	}

	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errors.HandleError(http.StatusInternalServerError, errPolicyLoading, ctx) {
		return
	}

	resourceName := GetResourceNameFromPath(ctx.FullPath())

	_, errRemovingPolicy := casdoor.Enforcer.RemoveFilteredPolicy(1, "/api/v1/"+resourceName+"/"+id.String())
	if errors.HandleError(http.StatusInternalServerError, errRemovingPolicy, ctx) {
		return
	}

	ctx.JSON(http.StatusNoContent, "Done")
}
