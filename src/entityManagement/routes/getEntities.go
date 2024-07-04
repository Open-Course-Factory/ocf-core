package controller

import (
	"net/http"
	"reflect"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

func (genericController genericController) GetEntities(ctx *gin.Context) {

	entitiesDto, shouldReturn1 := genericController.getEntities(ctx)
	if shouldReturn1 {
		return
	}

	ctx.JSON(http.StatusOK, entitiesDto)
}

func (genericController genericController) getEntities(ctx *gin.Context) ([]interface{}, bool) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entitiesDto, shouldReturn := genericController.getEntitiesFromName(entityName)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return nil, true
	}
	return entitiesDto, false
}

func (genericController genericController) getEntitiesFromName(entityName string) ([]interface{}, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, err := genericController.genericService.GetEntities(entityModelInterface)

	if err != nil {
		return nil, true
	}

	var entitiesDto []interface{}

	for _, page := range allEntitiesPages {

		entityModel := reflect.SliceOf(reflect.TypeOf(entityModelInterface))

		pageValue := reflect.ValueOf(page)

		if pageValue.Type().ConvertibleTo(entityModel) {
			convertedPage := pageValue.Convert(entityModel)

			for i := 0; i < convertedPage.Len(); i++ {

				item := convertedPage.Index(i).Interface()

				var shouldReturn bool
				entitiesDto, shouldReturn = genericController.appendEntityFromResult(entityName, item, entitiesDto)
				if shouldReturn {
					return nil, true
				}
			}
		} else {
			return nil, true
		}

	}
	return entitiesDto, false
}
