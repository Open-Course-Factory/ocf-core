package controller

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

func (genericController genericController) GetEntities(ctx *gin.Context) {

	entitiesDto, getEntityError := genericController.getEntities(ctx)
	if errors.HandleError(http.StatusNotFound, getEntityError, ctx) {
		return
	}

	ctx.JSON(http.StatusOK, entitiesDto)
}

func (genericController genericController) getEntities(ctx *gin.Context) ([]interface{}, error) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entitiesDto, shouldReturn := genericController.getEntitiesFromName(entityName)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return nil, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		}
	}
	return entitiesDto, nil
}

func (genericController genericController) getEntitiesFromName(entityName string) ([]interface{}, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, err := genericController.genericService.GetEntities(entityModelInterface)

	if err != nil {
		fmt.Println(err.Error())
		return nil, true
	}

	entitiesDto, shouldReturn := genericController.genericService.GetDtoArrayFromEntitiesPages(allEntitiesPages, entityModelInterface, entityName)
	if shouldReturn {
		return nil, true
	}
	return entitiesDto, false
}
