package controller

import (
	"net/http"

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
	entityModelInterface := GetEntityModelInterface(entityName)

	errorDelete := genericController.genericService.DeleteEntity(id, entityModelInterface)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Role not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
