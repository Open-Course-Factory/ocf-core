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
	if parseErr != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseErr.Error(),
		})
		return
	}

	entityName := GetEntityNameFromPath(ctx.FullPath())
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	entity, getEntityError := genericController.genericService.GetEntity(id, entityModelInterface)

	if getEntityError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entity not found",
		})
		return
	}

	errorDelete := genericController.genericService.DeleteEntity(id, entity)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entity not found",
		})
		return
	}

	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errPolicyLoading != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Policy not Found",
		})
		return
	}

	resourceName := GetResourceNameFromPath(ctx.FullPath())

	_, errRemovingPolicy := casdoor.Enforcer.RemoveFilteredPolicy(1, "/api/v1/"+resourceName+"/"+id.String())
	if errRemovingPolicy != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Policy not added",
		})
	}

	ctx.JSON(http.StatusNoContent, "Done")
}
