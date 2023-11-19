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

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	entityName := GetEntityNameFromPath(ctx.FullPath())
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)

	entity, entityError := genericController.genericService.GetEntity(id, entityModelInterface)

	if entityError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotAcceptable,
			ErrorMessage: err.Error(),
		})
		return
	}

	funcName := entityName + "ModelTo" + entityName + "Output"
	entityModel := reflect.TypeOf(entityModelInterface)
	entityValue := reflect.ValueOf(entity)

	var entityDto interface{}

	if entityValue.Elem().Type().ConvertibleTo(entityModel) {
		convertedEntity := entityValue.Elem().Convert(entityModel)

		item := convertedEntity.Interface()

		var errEntityDto bool
		entityDto, errEntityDto = genericController.getEntityFromResult(funcName, item)

		if errEntityDto {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Not Found",
			})
			return
		}
	}

	ctx.JSON(http.StatusOK, entityDto)
}
