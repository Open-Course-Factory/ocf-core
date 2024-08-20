package controller

import (
	"net/http"
	"reflect"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (genericController genericController) GetEntity(ctx *gin.Context) {

	id, err := uuid.Parse(ctx.Param("id"))
	if errors.HandleError(http.StatusBadRequest, err, ctx) {
		return
	}

	entityName := GetEntityNameFromPath(ctx.FullPath())
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	entity, entityError := genericController.genericService.GetEntity(id, entityModelInterface)
	if errors.HandleError(http.StatusNotFound, entityError, ctx) {
		return
	}

	entityModel := reflect.TypeOf(entityModelInterface)
	entityValue := reflect.ValueOf(entity)
	var entityDto interface{}

	if entityValue.Elem().Type().ConvertibleTo(entityModel) {
		convertedEntity := entityValue.Elem().Convert(entityModel)

		item := convertedEntity.Interface()

		var errEntityDto bool
		entityDto, errEntityDto = genericController.genericService.GetEntityFromResult(entityName, item)

		if errEntityDto {
			errors.HandleError(http.StatusNotFound, &errors.APIError{ErrorMessage: "Entity Not Found"}, ctx)
			return
		}
	}

	ctx.JSON(http.StatusOK, entityDto)
}
